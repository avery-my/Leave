package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// 配置按启动时加载的实例传递，各组件只接收需要的部分。
type SessionConfig struct {
	StudentTTL time.Duration `mapstructure:"student_ttl"`
	AdminTTL   time.Duration `mapstructure:"admin_ttl"`
}

type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Database string `mapstructure:"database"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
}
type RedisConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}
type AppConfig struct {
	Namespace  string `mapstructure:"namespace"`
	PackageDir string `mapstructure:"package_dir"`
	Port       int    `mapstructure:"port"`
}
type StorageConfig struct {
	RootDir string `mapstructure:"root_dir"`
	BaseURL string `mapstructure:"base_url"`
}
type LogConfig struct {
	Level    string `mapstructure:"level"`
	Encoding string `mapstructure:"encoding"`
	Dir      string `mapstructure:"dir"`
}
type SchoolConfig struct {
	URL      string
	Password string
}

func (c SchoolConfig) Enabled() bool {
	return c.URL != "" && c.Password != ""
}

type Config struct {
	School  *SchoolConfig `mapstructure:"-"` // nil 表示未配置 School。
	Session SessionConfig `mapstructure:"session"`
	Redis   RedisConfig   `mapstructure:"redis"`
	Log     LogConfig     `mapstructure:"log"`
	Mysql   MySQLConfig   `mapstructure:"mysql"`
	App     AppConfig     `mapstructure:"app"`
	Storage StorageConfig `mapstructure:"storage"`
}

// Load 读取并校验配置，不维护包级配置实例。
func Load() (Config, error) {
	var cfg Config
	v := viper.New()
	if err := loadDotEnv(); err != nil {
		return Config{}, fmt.Errorf("加载环境变量文件失败: %w", err)
	}

	configPath, err := FindConfigFile()
	if err != nil {
		return Config{}, err
	}
	v.SetConfigFile(configPath)
	v.AllowEmptyEnv(true)
	for _, binding := range [][2]string{
		{"mysql.password", "MYSQL_PASSWORD"},
		{"redis.password", "REDIS_PASSWORD"},
	} {
		if err := v.BindEnv(binding[0], binding[1]); err != nil {
			return Config{}, fmt.Errorf("绑定环境变量失败: %w", err)
		}
	}

	// 读取配置文件
	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("读取配置失败: %w", err)
	}

	// 将配置解析到cfg结构体
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置失败: %w", err)
	}
	cfg.School, err = loadSchoolConfig()
	if err != nil {
		return Config{}, err
	}
	cfg.Redis.Host = strings.TrimSpace(cfg.Redis.Host)
	cfg.App.Namespace = strings.TrimSpace(cfg.App.Namespace)
	cfg.App.PackageDir = strings.TrimSpace(cfg.App.PackageDir)
	cfg.Storage.RootDir = strings.TrimSpace(cfg.Storage.RootDir)
	cfg.Storage.BaseURL = strings.TrimSpace(cfg.Storage.BaseURL)

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) validate() error {
	if err := c.Redis.validate(); err != nil {
		return err
	}
	if err := c.Mysql.validate(); err != nil {
		return err
	}
	if err := c.App.validate(); err != nil {
		return err
	}
	if err := c.Session.validate(); err != nil {
		return err
	}
	return c.Storage.validate()
}

func (c RedisConfig) validate() error {
	if c.Host == "" {
		return fmt.Errorf("redis.host cannot be empty")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("redis.port must be between 1 and 65535")
	}
	if c.DB < 0 {
		return fmt.Errorf("redis.db cannot be negative")
	}
	return nil
}

func (c MySQLConfig) validate() error {
	if c.Password == "" {
		return fmt.Errorf("MySQL password cannot be empty; configure MYSQL_PASSWORD")
	}
	return nil
}

func (c AppConfig) validate() error {
	if c.Namespace == "" || strings.ContainsAny(c.Namespace, ": \t\r\n") {
		return fmt.Errorf("app.namespace must be nonempty and contain no colon or whitespace")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("app.port must be between 1 and 65535")
	}
	return nil
}

func (c SessionConfig) validate() error {
	if c.StudentTTL <= 0 || c.AdminTTL <= 0 {
		return fmt.Errorf("session TTLs must be positive durations")
	}
	return nil
}

func (c StorageConfig) validate() error {
	if c.RootDir == "" {
		return fmt.Errorf("storage.root_dir cannot be empty")
	}
	if c.BaseURL == "" {
		return fmt.Errorf("storage.base_url cannot be empty")
	}
	return nil
}

func loadSchoolConfig() (*SchoolConfig, error) {
	school := SchoolConfig{
		URL:      strings.TrimRight(strings.TrimSpace(os.Getenv("SCHOOL_URL")), "/"),
		Password: os.Getenv("SCHOOL_PASSWORD"),
	}
	if school == (SchoolConfig{}) {
		return nil, nil
	}

	site, err := url.Parse(school.URL)
	if err != nil || site.Scheme != "https" ||
		site.Hostname() == "" || site.User != nil || site.Path != "" ||
		site.ForceQuery || site.RawQuery != "" || strings.Contains(school.URL, "#") {
		return nil, fmt.Errorf("SCHOOL_URL must be an HTTPS root URL")
	}
	if !school.Enabled() {
		return nil, fmt.Errorf("SCHOOL_PASSWORD cannot be empty when SCHOOL_URL is configured")
	}
	return &school, nil
}

// FindConfigFile 为服务和迁移工具选择同一个环境配置，不匹配模板或任意 YAML。
func FindConfigFile() (string, error) {
	// 宝塔会从程序所在目录启动；兼容新旧发布布局及根目录启动。
	paths := []string{
		"./config/config.local.yaml",
		"./config/config.test.yaml",
		"./config/config.server.yaml",
		"../../config/config.test.yaml",
		"../../config/config.server.yaml",
		"../../../config/config.test.yaml",
		"../../../config/config.server.yaml",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("检查配置文件 %q 失败: %w", path, err)
		}
	}
	return "", fmt.Errorf("未找到配置文件，请提供 config.local.yaml、config.test.yaml 或 config.server.yaml")
}

func loadDotEnv() error {
	// current 会解析为 releases/vN 或 backend/releases/vN，分别向上两级或三级。
	for _, path := range []string{".env", "../.env", "../../.env", "../../../.env"} {
		if _, err := os.Stat(path); err == nil {
			return gotenv.Load(path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
