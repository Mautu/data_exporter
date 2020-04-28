# data_exporter
数据源数据监控  
支持的数据源:  
## mysql  
* 数据源配置
```
datasource:
  mysql:
    test2: user:password@tcp(localhost:5555)/dbname
```
## elasticsearch
* 数据源配置
```
datasource:
  elasticsearch:
    test1: http://127.0.0.1:9200/indexname-2019
```
## redis
* 数据源配置
```
  redis:
    test1: password|127.0.0.1:6379
```
## 查询配置
```
  - name: es #指标名称
    datasource: test1 #数据源名称 对应数据源配置
    sourcetype: elasticsearch #数据源类型  mysql|elasticsearch|redis
    query: ERROR
    #查询条件 获取的值转化为float64失败会默认设置为0,es查询数据为最近10m内数据
    #mysql为查询语句,监控指标值为value字段需设定字段别名为”value”
    #elastic为查询表达式,查询的是对应匹配数据的行数
    #redis为key,不支持keys
    constant_lables:
    #固定标签
      project: hemu
      service: test
    variable_lables:
      - state
    #可变标签随数据变化,仅支持mysql,elaticsearch,redis配置后会自动忽略
```    