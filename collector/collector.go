package collector

import (
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mautu/data_exporter/conf"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gomodule/redigo/redis"
	"github.com/prometheus/client_golang/prometheus"
	plog "github.com/prometheus/common/log"
	"github.com/tidwall/gjson"
	"github.com/xormplus/xorm"
	"go.uber.org/zap"
)

// Result es返回值结构体
type Result struct {
	aggs aggregations `json:"aggregations"`
	hits hits         `json:"hits"`
}
type aggregations struct {
	Type Type `json:"type"`
}
type Type struct {
	other_doc_count float64 `json:"sum_other_doc_count"`
	bukets          []*agg  `json:"buckets"`
}
type agg struct {
	key    string  `json:"key"`
	count  float64 `json:"doc_count"`
	bukets []*agg  `json:"buckets"`
}
type hits struct {
	total float64 `json:"total"`
}
type MetricsQuery interface {
	SetMetric(me Metrics, ch chan<- prometheus.Metric)
}

func (me *Metrics) SetElasticValuebygjson(index int, Q conf.Query, ch chan<- prometheus.Metric) {
	//ConstantLables := strings.Split(me.ConstantLables[Q.Name], "|")
	Constantvalue := []string{}
	Variablevalue := []string{}
	if me.Constantvalue[index] != "" {
		Constantvalue = strings.Split(me.Constantvalue[index], "|")
	}
	//VariableLables := strings.Split(me.VariableLables[Q.Name], "|")
	if me.VariableLables[index] != "" {
		Variablevalue = strings.Split(me.VariableLables[index], "|")
	}
	conn, ok := me.conf.Datasource.Elastics[Q.Datasource]
	if ok && Q.Sourcetype == "elasticsearch" && (len(Variablevalue) <= 2 || me.VariableLables[index] == "") {
		post := Q.Info
		elasticurl := conn
		aggstring := ""
		for key := range Variablevalue {
			aggstring = `,"aggregations":{"type":{"terms":{"field":"` + Variablevalue[len(Variablevalue)-key-1] + `"}` + aggstring + `}}`
		}
		//data := `{"size": 0,"query":{"bool":{"must":{"query_string":{"query":"` + Q.Info + `"}},"filter":{"range":{"@timestamp":{"gte":"now-15m","lt":"now"}}}}},` + `"aggregations":{"type":{"terms":{"field":"_type"},}}` + aggstring + `}`
		data := `{"size": 0,"query":{"bool":{"must":{"query_string":{"query":"` + Q.Info + `"}},"filter":{"range":{"@timestamp":{"gte":"now-10m","lt":"now"}}}}}` + aggstring + `}`
		//plog.Infoln(data)
		url := elasticurl + "/_search"
		//req, err := http.NewRequest("GET", url, bytes.NewBuffer([]byte(data)))
		req, err := http.NewRequest("GET", url, strings.NewReader(data))
		//req:=HtttpRe
		req.Header.Set("Content-Type", "application/json;charset=utf-8;")
		res, err := me.httpclient.Do(req)
		if err != nil {
			plog.Errorln("connection error", zap.String("conn", elasticurl), zap.String("error", err.Error()))
			return
		}
		defer res.Body.Close()
		result, err := ioutil.ReadAll(res.Body)
		//err = json.Unmarshal([]byte(body), reslut)
		//result, err := gjson.DecodeToJson([]byte(body))
		plog.Infoln(Q.Info)
		plog.Infoln(data)
		plog.Infoln(string(result))
		if err != nil {
			plog.Errorln("reque encode error", zap.String("conn", elasticurl), zap.String("query", post), zap.String("error", err.Error()))
		}
		if len(Variablevalue) == 0 {
			ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, gjson.GetBytes(result, "hits.total").Float(), Constantvalue...)
		} else if len(Variablevalue) == 1 {
			Lables := append(Constantvalue, "other")
			ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, gjson.GetBytes(result, "aggregations.type.sum_other_doc_count").Float(), Lables...)
			aggs := gjson.GetBytes(result, "aggregations.type.buckets.#.key")
			//plog.Infoln("1 lables", len(aggs.Array()))
			for _, name := range aggs.Array() {
				lable := name.String()
				Lables := append(Constantvalue, lable)
				count := gjson.GetBytes(result, `aggregations.type.buckets.#[key=="`+lable+`"].doc_count`).Float()
				plog.Infoln(Lables, count)
				ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, count, Lables...)
			}

		} else if len(Variablevalue) == 2 {
			plog.Infoln(string(result))
			aggs := gjson.GetBytes(result, "aggregations.type.buckets.#.key").Array()
			//plog.Infoln("2 lables", len(aggs))
			for _, name := range aggs {
				lable := name.String()
				Lables := append(Constantvalue, lable)
				Lablesother := append(Lables, "other")
				count := gjson.GetBytes(result, `aggregations.type.buckets.#[key=="`+lable+`"].type.sum_other_doc_count`).Float()
				ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, count, Lablesother...)
				aggs2 := gjson.GetBytes(result, `aggregations.type.buckets.#[key=="`+lable+`"].type.buckets.#.key`).Array()
				for _, name2 := range aggs2 {
					lable2 := name2.String()
					Lables := append(Lables, lable2)
					count2 := gjson.GetBytes(result, `aggregations.type.buckets.#[key=="`+lable+`"].type.buckets.#[key=="`+lable2+`"].doc_count`).Float()
					plog.Infoln(Lables, count2)
					ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, count2, Lables...)
				}
			}
		}
	} else {

		plog.Infoln("error Query", ok, Q.Sourcetype, Q.Name, len(Variablevalue), me.VariableLables[index], len(me.VariableLables[index]))
	}
}

// Metrics 指标结构体
type Metrics struct {
	metrics        map[string]*prometheus.Desc
	querymetric    map[int]string
	ConstantLables map[int]string
	Constantvalue  map[int]string
	VariableLables map[int]string
	conf           *conf.Config
	mutex          sync.Mutex
	mysqls         map[string]*xorm.Engine
	redises        map[string]*redis.Pool
	httpclient     *http.Client
}

func (me *Metrics) setMysqlValue(index int, Q conf.Query, ch chan<- prometheus.Metric) {
	ConstantLables := []string{}
	Constantvalue := []string{}
	VariableLables := []string{}
	if me.ConstantLables[index] != "" {
		ConstantLables = strings.Split(me.ConstantLables[index], "|")
		Constantvalue = strings.Split(me.Constantvalue[index], "|")
	}
	if me.VariableLables[index] != "" {
		VariableLables = strings.Split(me.VariableLables[index], "|")
	}
	//Variablevalue := strings.Split(me.Variablevalue[Q.Name], "|")
	Lables := []string{}
	for i, _ := range ConstantLables {
		Lables = append(Lables, Constantvalue[i])
	}
	if Q.Sourcetype == "mysql" {
		conn := me.mysqls[Q.Datasource]
		rows, err := conn.QueryResult(Q.Info).List()
		if err != nil {
			plog.Errorln("error", zap.String("sql", Q.Info), zap.String("error", err.Error()))
		}
		for _, row := range rows {
			Lablesrow := Lables
			for _, lable := range VariableLables {
				Lablesrow = append(Lablesrow, string(row[lable]))
			}
			value, err := strconv.ParseFloat(string(row["value"]), 64)
			if err == nil {
				ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, value, Lablesrow...)
			} else {
				plog.Errorln("value can't to float64 set to 0", zap.String("Datasource", Q.Name), zap.Error(err))
				ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, 0.0, Lablesrow...)
			}
		}
	} else {
		plog.Errorln("datasource is not mysql", zap.String("Datasource", Q.Datasource))
	}
}
func (me *Metrics) setRedisValue(index int, Q conf.Query, ch chan<- prometheus.Metric) {
	ConstantLables := strings.Split(me.ConstantLables[index], "|")
	Constantvalue := strings.Split(me.Constantvalue[index], "|")
	Lables := []string{}
	for i, _ := range ConstantLables {
		Lables = append(Lables, Constantvalue[i])
	}
	if Q.Sourcetype == "redis" {
		c := me.redises[Q.Datasource].Get()
		defer c.Close()
		tmp, err := redis.String(c.Do("get", Q.Info))
		if err == nil {
			Metrics, ok := me.metrics[me.querymetric[index]]
			f, err := strconv.ParseFloat(tmp, 64)
			if err == nil && ok {
				ch <- prometheus.MustNewConstMetric(Metrics, prometheus.GaugeValue, f, Lables...)
			} else if !ok {
				plog.Info("metrics not find", zap.Bool("ok", ok), zap.Int("index", index), zap.String("query", me.querymetric[index]))
			} else {
				plog.Errorln("value can't to float64,set to 0", zap.Error(err), zap.String("value", tmp))
				ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, 0.0, Lables...)
			}

		} else {
			plog.Errorln("get key error", zap.Error(err), zap.String("key", Q.Info))
			ch <- prometheus.MustNewConstMetric(me.metrics[me.querymetric[index]], prometheus.GaugeValue, 0.0, Lables...)
		}

	} else {
		plog.Errorln("datasource is not redis", zap.String("Datasource", Q.Datasource))
	}
}

/**
 * 函数：newGlobalMetric
 * 功能：创建指标描述符
 */
func newGlobalMetric(metricName string, docString string, labels []string) *prometheus.Desc {
	return prometheus.NewDesc(metricName, docString, labels, nil)
}

// Describe 功能：传递结构体中的指标描述符到channel
func (c *Metrics) Describe(ch chan<- *prometheus.Desc) {
	for _, m := range c.metrics {
		ch <- m
	}
}

// NewMetrics 功能：初始化指标
func NewMetrics(c *conf.Config) *Metrics {
	//指标列表
	metriclist := make(map[string]*prometheus.Desc)
	querymetric := make(map[int]string)
	//静态标签
	ConstantLables := make(map[int]string)
	//静态标签值
	Constantvalue := make(map[int]string)
	//动态标签
	VariableLables := make(map[int]string)

	for i, query := range c.Querys {
		constLables := []string{}
		constvalue := []string{}
		variableLables := []string{}
		lables := []string{}
		for key, value := range query.ConstantLables {
			constLables = append(constLables, key)
			constvalue = append(constvalue, value)
			lables = append(lables, key)
		}
		if query.Sourcetype != "redis" {
			for _, value := range query.VariableLables {
				variableLables = append(variableLables, value)
				if query.Sourcetype == "elasticsearch" && strings.Contains(value, ".keyword") {
					value = strings.Replace(value, ".keyword", "", -1)
				}
				lables = append(lables, value)
			}
		}
		_, ok := metriclist[query.Name]
		if !ok {
			metriclist[query.Name] = newGlobalMetric(query.Name, query.Info, lables)
		}
		querymetric[i] = query.Name
		ConstantLables[i] = strings.Join(constLables, "|")
		Constantvalue[i] = strings.Join(constvalue, "|")
		VariableLables[i] = strings.Join(variableLables, "|")
		plog.Infoln("query:", query.Name)
		plog.Infoln("ConstantLables:", ConstantLables[i])
		plog.Infoln("Constantvalue:", Constantvalue[i])
		plog.Infoln("VariableLables:", VariableLables[i])

	}
	mysqls := make(map[string]*xorm.Engine)
	redises := make(map[string]*redis.Pool)
	for key, value := range c.Datasource.Mysqls {
		Mysql, err := xorm.NewEngine("mysql", value+"?charset=utf8")
		if err != nil {
			plog.Errorln("数据库连接失败",
				zap.String("mysql", value+"?charset=utf8"),
				zap.String("error", err.Error()),
				zap.String("time", time.Now().Format("2006-01-02 15:04:05")))
		} else {
			mysqls[key] = Mysql
		}
	}
	for key, value := range c.Datasource.Redis {
		password := ""
		conn := value
		if strings.Count(value, "|") == 1 {
			password = strings.Split(value, "|")[0]
			conn = strings.Split(value, "|")[1]
		}
		Redis := &redis.Pool{ //实例化一个连接池
			MaxIdle: 16, //最初的连接数量
			// MaxActive:1000000,    //最大连接数量
			MaxActive:   0,   //连接池最大连接数量,不确定可以用0（0表示自动定义），按需分配
			IdleTimeout: 300, //连接关闭时间 300秒 （300秒不使用自动关闭）
			Dial: func() (redis.Conn, error) { //要连接的redis数据库
				return redis.Dial("tcp", conn, redis.DialKeepAlive(1*time.Second), redis.DialPassword(password), redis.DialConnectTimeout(5*time.Second), redis.DialReadTimeout(1*time.Second), redis.DialWriteTimeout(1*time.Second))
			},
		}
		redises[key] = Redis

	}
	return &Metrics{
		conf:           c,
		metrics:        metriclist,
		querymetric:    querymetric,
		ConstantLables: ConstantLables,
		Constantvalue:  Constantvalue,
		VariableLables: VariableLables,
		httpclient: &http.Client{
			Timeout: 10 * time.Second,
		},
		redises: redises,
		mysqls:  mysqls,
	}
}

// Collect 功能：抓取最新的数据，传递给channel
func (c *Metrics) Collect(ch chan<- prometheus.Metric) {
	var wg = sync.WaitGroup{}
	getmetrics := func(index int, query conf.Query) {
		defer wg.Done()
		if query.Sourcetype == "elasticsearch" {
			c.SetElasticValuebygjson(index, query, ch)
		} else if query.Sourcetype == "mysql" {
			c.setMysqlValue(index, query, ch)
		} else if query.Sourcetype == "redis" {
			c.setRedisValue(index, query, ch)
		}
		c.mutex.Lock() // 加锁
		defer c.mutex.Unlock()
	}
	for index, query := range c.conf.Querys {
		wg.Add(1)
		go getmetrics(index, query)
	}
	wg.Wait()

}
