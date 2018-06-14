package mpawsecs

import (
	"errors"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	mp "github.com/mackerelio/go-mackerel-plugin"
)

const (
	namespace              = "AWS/ECS"
	metricsTypeAverage     = "Average"
	metricsTypeMinimum     = "Minimum"
	metricsTypeMaximum     = "Maximum"
	metricsTypeSampleCount = "SampleCount"
)

type metrics struct {
	Name string
	Type string
}

// ECSPlugin mackerel plugin for ecs
type ECSPlugin struct {
	AccessKeyID     string
	SecretAccessKey string
	CloudWatch      *cloudwatch.CloudWatch
	ClusterName     string
	ServiceName     string
	Prefix          string
	Region          string
}

// MetricKeyPrefix interface for PluginWithPrefix
func (p ECSPlugin) MetricKeyPrefix() string {
	if p.Prefix == "" {
		p.Prefix = "ECS"
	}
	return p.Prefix
}

func (p *ECSPlugin) prepare() error {
	sess, err := session.NewSession()
	if err != nil {
		return err
	}

	config := aws.NewConfig()
	if p.AccessKeyID != "" && p.SecretAccessKey != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(p.AccessKeyID, p.SecretAccessKey, ""))
	}
	config = config.WithRegion(p.Region)

	p.CloudWatch = cloudwatch.New(sess, config)

	return nil
}

func (p ECSPlugin) getLastPoint(metric metrics) (float64, error) {
	now := time.Now()

	dimensions := []*cloudwatch.Dimension{
		{
			Name:  aws.String("ClusterName"),
			Value: aws.String(p.ClusterName),
		},
	}
	if p.ServiceName != "" {
		dimensions = append(dimensions, &cloudwatch.Dimension{
			Name:  aws.String("ServiceName"),
			Value: aws.String(p.ServiceName),
		})
	}

	response, err := p.CloudWatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: dimensions,
		StartTime:  aws.Time(now.Add(time.Duration(180) * time.Second * -1)), // 3 min (to fetch at least 1 data-point)
		EndTime:    aws.Time(now),
		MetricName: aws.String(metric.Name),
		Period:     aws.Int64(60),
		Statistics: []*string{aws.String(metric.Type)},
		Namespace:  aws.String(namespace),
	})
	if err != nil {
		return 0, err
	}

	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return 0, errors.New("fetched no datapoints")
	}

	// get a least recently datapoint
	// because a most recently datapoint is not stable.
	least := time.Now()
	var latestVal float64
	for _, dp := range datapoints {
		if dp.Timestamp.Before(least) {
			least = *dp.Timestamp
			switch metric.Type {
			case metricsTypeAverage:
				latestVal = *dp.Average
			case metricsTypeMinimum:
				latestVal = *dp.Minimum
			case metricsTypeMaximum:
				latestVal = *dp.Maximum
			case metricsTypeSampleCount:
				latestVal = *dp.SampleCount
			}
		}
	}

	return latestVal, nil
}

// FetchMetrics fetch the metrics
func (p ECSPlugin) FetchMetrics() (map[string]float64, error) {
	stat := make(map[string]float64)

	for name := range p.GraphDefinition() {
		if name == "Task" {
			met := metrics{"CPUUtilization", metricsTypeSampleCount}
			v, err := p.getLastPoint(met)
			if err == nil {
				stat[name+"Running"] = v
			} else {
				log.Printf("%s: %s", met, err)
			}
			continue
		}

		for _, t := range []string{metricsTypeAverage, metricsTypeMinimum, metricsTypeMaximum} {
			met := metrics{name, t}
			v, err := p.getLastPoint(met)
			if err == nil {
				stat[name+t] = v
			} else {
				log.Printf("%s: %s", met, err)
			}
		}
	}

	return stat, nil
}

// GraphDefinition of ECSPlugin
func (p ECSPlugin) GraphDefinition() map[string]mp.Graphs {
	labelPrefix := strings.Title(p.Prefix)
	labelPrefix = strings.Replace(labelPrefix, "-", " ", -1)

	baseGraphs := map[string]mp.Graphs{
		"CPUUtilization": {
			Label: labelPrefix + " CPUUtilization",
			Unit:  "percentage",
			Metrics: []mp.Metrics{
				{Name: "CPUUtilizationAverage", Label: "Average"},
				{Name: "CPUUtilizationMinimum", Label: "Minimum"},
				{Name: "CPUUtilizationMaximum", Label: "Maximum"},
			},
		},
		"MemoryUtilization": {
			Label: labelPrefix + " MemoryUtilization",
			Unit:  "percentage",
			Metrics: []mp.Metrics{
				{Name: "MemoryUtilizationAverage", Label: "Average"},
				{Name: "MemoryUtilizationMinimum", Label: "Minimum"},
				{Name: "MemoryUtilizationMaximum", Label: "Maximum"},
			},
		},
	}
	if p.ServiceName != "" {
		baseGraphs["Task"] = mp.Graphs{
			Label: labelPrefix + " Task",
			Unit:  "integer",
			Metrics: []mp.Metrics{
				{Name: "TaskRunning", Label: "Running"},
			},
		}
		return baseGraphs
	}
	baseGraphs["CPUReservation"] = mp.Graphs{
		Label: labelPrefix + " CPUReservation",
		Unit:  "percentage",
		Metrics: []mp.Metrics{
			{Name: "CPUReservationAverage", Label: "Average"},
			{Name: "CPUReservationMinimum", Label: "Minimum"},
			{Name: "CPUReservationMaximum", Label: "Maximum"},
		},
	}
	baseGraphs["MemoryReservation"] = mp.Graphs{
		Label: labelPrefix + " MemoryReservation",
		Unit:  "percentage",
		Metrics: []mp.Metrics{
			{Name: "MemoryReservationAverage", Label: "Average"},
			{Name: "MemoryReservationMinimum", Label: "Minimum"},
			{Name: "MemoryReservationMaximum", Label: "Maximum"},
		},
	}
	return baseGraphs
}

// Do the plugin
func Do() {
	optAccessKeyID := flag.String("access-key-id", "", "AWS Access Key ID")
	optSecretAccessKey := flag.String("secret-access-key", "", "AWS Secret Access Key")
	optClusterName := flag.String("cluster-name", "", "Cluster name")
	optServiceName := flag.String("service-name", "", "Service name")
	optPrefix := flag.String("metric-key-prefix", "ECS", "Metric key prefix")
	optRegion := flag.String("region", "", "AWS region")
	flag.Parse()

	var plugin ECSPlugin

	plugin.AccessKeyID = *optAccessKeyID
	plugin.SecretAccessKey = *optSecretAccessKey
	plugin.ClusterName = *optClusterName
	plugin.ServiceName = *optServiceName
	plugin.Prefix = *optPrefix
	plugin.Region = *optRegion

	err := plugin.prepare()
	if err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(plugin)

	helper.Run()
}
