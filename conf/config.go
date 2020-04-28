package conf

import (
	"fmt"
	"io/ioutil"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	plog "github.com/prometheus/common/log"
	"go.uber.org/zap"
	yaml "gopkg.in/yaml.v2"
)

type Config struct {
	Version    string     `yaml:"version"`
	Port       int        `yaml:"port"`
	Datasource datasource `yaml:"datasource"`
	Querys     []Query    `yaml:"querys"`
}

type Conn struct {
	info string `yaml:"name"`
	Conn string `yaml:"conn"`
}
type datasource struct {
	Mysqls   map[string]string `yaml:"mysql"`
	Elastics map[string]string `yaml:"elasticsearch"`
	Redis    map[string]string `yaml:"redis"`
}
type Query struct {
	Name           string            `yaml:"name"`
	Datasource     string            `yaml:"datasource"`
	Sourcetype     string            `yaml:"sourcetype"`
	Info           string            `yaml:"query"`
	ConstantLables map[string]string `yaml:"constant_lables"`
	VariableLables []string          `yaml:"variable_lables"`
}

//Load 加载配置
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	queryerror := 0
	// 校验redis数据源连接串
	for name, redisconn := range cfg.Datasource.Redis {
		if strings.Count(redisconn, "|") > 1 {
			plog.Errorln("redis连接串存在多个密码分隔符|", zap.String("Name", name), zap.String("conn", redisconn))
			queryerror++
		}
	}
	//查询数据数据校验
	for _, query := range cfg.Querys {
		lables := []string{}
		for k, _ := range query.ConstantLables {
			lables = append(lables, k)
		}
		for _, v := range query.VariableLables {
			lables = append(lables, v)
		}
		for _, query1 := range cfg.Querys {
			if query.Name == query1.Name {
				lables1 := make(map[string]int)
				for k, _ := range query1.ConstantLables {
					lables1[k] = 1
				}
				for _, v := range query1.VariableLables {
					lables1[v] = 1
				}
				if len(lables) != len(lables1) {
					queryerror++
				}
				for _, key := range lables {
					_, ok := lables1[key]
					if !ok {
						plog.Errorln("the same query name have diffrent lables ", zap.String("query", query.Name), zap.String("lables", key))
						queryerror++
					}
				}
			}
		}
		//校验redis查询key,不支持*
		if query.Sourcetype == "redis" {
			if strings.Count(query.Info, "*") != 0 {
				plog.Errorln("redis查询key中存在*模糊匹配,不支持", zap.String("Name", query.Name), zap.String("query", query.Info))
				queryerror++
			}
		}
	}
	if queryerror > 0 {
		plog.Fatalln("the same query name have diffrent lables ", zap.Int("errorcount", queryerror))
	}
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(string(content))
	if err != nil {
		return nil, fmt.Errorf("parsing YAML file %s: %v", filename, err)
	}
	return cfg, nil
}
