package main

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
)

type Target struct {
	ID string `toml:"id" json:"id"`
	Name string `toml:"name" json:"name"`
	Campus string `toml:"campus" json:"campus"`
}

type Profile struct {
	Department string `toml:"depart_name"`
	Type string `toml:"user_type"`
	Sex string `toml:"sex"`
	Size string `toml:"size"`
	IDNumber string `toml:"id_number"`
	Phone string `toml:"phone"`
	Emergency string `toml:"emergency_contact"`
}

type Config struct {
	Targets []Target `toml:"targets"`
	Monitor struct {
		Username string `toml:"username"`
		Interval float64 `toml:"poll_interval_seconds"`
		RequestTimeout float64 `toml:"request_timeout_seconds"`
		LoginTimeout float64 `toml:"login_timeout_seconds"`
		Headless bool `toml:"headless"`
	} `toml:"monitor"`
	Browser struct {
		Kind string `toml:"kind"`
		Executable string `toml:"executable_path"`
	} `toml:"browser"`
	Profile Profile `toml:"profile"`
}

var configPath = "config.toml"
var runtimeRoot = "runtime"
var validID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
var phonePattern = regexp.MustCompile(`^1[3-9][0-9]{9}$`)
var identityPattern = regexp.MustCompile(`^[1-9][0-9]{5}(18|19|20)[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])[0-9]{3}[0-9Xx]$`)
var sizes = []string{"XS(160/84A)", "S(165/88A)", "M(170/92A)", "L(175/96A)", "XL(180/100A)", "2XL(185/104A)", "3XL(190/108A)"}

func loadConfig() (Config, error) {
	c := Config{}
	c.Monitor.Interval, c.Monitor.RequestTimeout, c.Monitor.LoginTimeout = 30, 30, 600
	c.Monitor.Headless = true
	c.Profile.Type = "学生"
	b, err := os.ReadFile(configPath)
	if err != nil { return c, fmt.Errorf("无法读取配置文件：%w", err) }
	d := toml.NewDecoder(strings.NewReader(string(b))).DisallowUnknownFields()
	if err := d.Decode(&c); err != nil { return c, fmt.Errorf("TOML 配置格式或字段错误") }
	c.normalize()
	if err := c.validate(); err != nil { return c, err }
	return c, nil
}

func (c *Config) normalize() {
	c.Monitor.Username = strings.TrimSpace(c.Monitor.Username)
	c.Browser.Kind = strings.TrimSpace(c.Browser.Kind)
	c.Browser.Executable = strings.TrimSpace(c.Browser.Executable)
	for i := range c.Targets {
		c.Targets[i].ID = strings.TrimSpace(c.Targets[i].ID)
		c.Targets[i].Name = strings.TrimSpace(c.Targets[i].Name)
		c.Targets[i].Campus = strings.TrimSpace(c.Targets[i].Campus)
	}
	v := reflect.ValueOf(&c.Profile).Elem()
	for i := 0; i < v.NumField(); i++ { v.Field(i).SetString(strings.TrimSpace(v.Field(i).String())) }
}

func (c Config) validate() error {
	for key, value := range map[string]float64{"poll_interval_seconds": c.Monitor.Interval, "request_timeout_seconds": c.Monitor.RequestTimeout, "login_timeout_seconds": c.Monitor.LoginTimeout} {
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) { return fmt.Errorf("%s 必须为有限正数", key) }
	}
	if c.Browser.Kind != "" && c.Browser.Kind != "chrome" && c.Browser.Kind != "edge" { return fmt.Errorf("browser.kind 必须为 chrome 或 edge") }
	if c.Profile.Type != "学生" && c.Profile.Type != "教职工" { return fmt.Errorf("user_type 必须为学生或教职工") }
	if c.Profile.Sex != "" && c.Profile.Sex != "男" && c.Profile.Sex != "女" { return fmt.Errorf("sex 必须为男或女") }
	seen := map[string]bool{}
	for _, t := range c.Targets {
		if !validID.MatchString(t.ID) || strings.TrimSpace(t.Name) == "" || (t.Campus != "西校区" && t.Campus != "高新校区") { return fmt.Errorf("目标比赛编号、名称或校区无效") }
		if seen[t.ID] { return fmt.Errorf("目标比赛编号重复") }
		seen[t.ID] = true
	}
	return nil
}

func (p Profile) complete() error {
	v := reflect.ValueOf(p)
	for i := 0; i < v.NumField(); i++ {
		if strings.TrimSpace(v.Field(i).String()) == "" { return fmt.Errorf("请填写 profile.%s", v.Type().Field(i).Tag.Get("toml")) }
	}
	found := false
	for _, s := range sizes { found = found || p.Size == s }
	if !found { return fmt.Errorf("size 必须填写网站使用的完整衣服尺码") }
	if !phonePattern.MatchString(p.Phone) || !phonePattern.MatchString(p.Emergency) { return fmt.Errorf("手机和紧急联系人电话必须为有效的十一位号码") }
	if !identityPattern.MatchString(p.IDNumber) { return fmt.Errorf("id_number 不符合十八位身份证号格式") }
	return nil
}

func (c Config) requireWatch() error {
	if len(c.Targets) == 0 { return fmt.Errorf("请运行 select 选择目标比赛") }
	if c.Monitor.Username == "" { return fmt.Errorf("请填写 monitor.username 或运行 credentials set") }
	return c.Profile.complete()
}

func updateConfig(key []string, value any) error {
	b, err := os.ReadFile(configPath)
	if err != nil { return err }
	d, err := tomledit.Parse(bytes.NewReader(b))
	if err != nil { return fmt.Errorf("配置文件解析失败") }
	var encoded bytes.Buffer
	if err := toml.NewEncoder(&encoded).SetTablesInline(true).Encode(map[string]any{"value": value}); err != nil { return err }
	valueDoc, err := tomledit.Parse(&encoded)
	if err != nil { return fmt.Errorf("配置值编码失败") }
	datum := valueDoc.First("value").KeyValue.Value.X
	entry := d.First(key...)
	if entry != nil && entry.KeyValue != nil { entry.KeyValue.Value.X = datum } else {
		if len(key) == 1 {
			var sections []*tomledit.Section
			for _, section := range d.Sections { if !parser.Key(key).IsPrefixOf(section.TableName()) { sections = append(sections, section) } }
			d.Sections = sections
		}
		var section *tomledit.Section
		if len(key) == 1 {
			if d.Global == nil { d.Global = &tomledit.Section{} }
			section = d.Global
		} else {
			parent := d.First(key[:len(key)-1]...)
			if parent != nil { section = parent.Section } else {
				section = &tomledit.Section{Heading: &parser.Heading{Name: parser.Key(key[:len(key)-1])}}
				d.Sections = append(d.Sections, section)
			}
		}
		section.Items = append(section.Items, &parser.KeyValue{Name: parser.Key{key[len(key)-1]}, Value: parser.Value{X: datum}})
	}
	var output bytes.Buffer
	if err := tomledit.Format(&output, d); err != nil { return err }
	var c Config
	c.Monitor.Interval, c.Monitor.RequestTimeout, c.Monitor.LoginTimeout = 30, 30, 600
	c.Profile.Type = "学生"
	if err := toml.NewDecoder(bytes.NewReader(output.Bytes())).DisallowUnknownFields().Decode(&c); err != nil { return fmt.Errorf("更新配置失败") }
	c.normalize()
	if err := c.validate(); err != nil { return err }
	// 验证完整配置后原子替换，保留原有注释和字段顺序。
	pending := configPath + ".pending"
	if err := os.WriteFile(pending, output.Bytes(), 0600); err != nil { return err }
	return os.Rename(pending, configPath)
}

func stableConfig(expected Config) error {
	current, err := loadConfig()
	if err != nil { return err }
	if !reflect.DeepEqual(current.Targets, expected.Targets) || current.Profile != expected.Profile || current.Monitor.Username != expected.Monitor.Username || current.Browser != expected.Browser {
		return fmt.Errorf("账号、比赛、报名资料或浏览器配置发生变化，请重新启动")
	}
	return current.Profile.complete()
}

func setRoot(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil { return err }
	configPath = abs
	runtimeRoot = filepath.Join(filepath.Dir(abs), "runtime")
	return nil
}
