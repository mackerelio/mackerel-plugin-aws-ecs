# mackerel-plugin-aws-ecs

## Install

```sh
% mkr plugin install mackerelio/mackerel-plugin-aws-ecs
```

## Setting

```
[plugin.metrics.aws-ecs]
command = "/path/to/mackerel-plugin-aws-ecs -access-key-id XXX -secret-access-key YYY -metric-key-prefix MyECS -cluster-name MyClusterName -service-name MyServiceName -region ap-northeast-1"
```
