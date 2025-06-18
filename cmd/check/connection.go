package check

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/go-redis/redis/v8"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"google.golang.org/api/option"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check cli",
	}

	cmd.AddCommand(newConnection())

	return cmd
}

type Connection interface {
	CheckConnection() error
	GetDescription() string
}

type Resource struct {
	Type      string
	IsChecked bool
	Config    Connection
}

// HTTPConfig represents configuration for HTTP connection
type HTTPConfig struct {
	URL     string
	Method  string
	Timeout time.Duration
}

func (c *HTTPConfig) CheckConnection() error {
	client := &http.Client{
		Timeout: c.Timeout,
	}

	resp, err := client.Get(c.URL)
	if err != nil {
		return fmt.Errorf("HTTP connection failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}
	return nil
}

func (c *HTTPConfig) GetDescription() string {
	return c.Method + ":" + c.URL
}

// MySQLConfig represents configuration for MySQL connection
type MySQLConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

func (c *MySQLConfig) CheckConnection() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?timeout=5s",
		c.User, c.Password, c.Host, c.Port, c.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to MySQL: %v", err)
	}
	defer db.Close()

	// Test the connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("MySQL ping failed: %v", err)
	}
	return nil
}

func (c *MySQLConfig) GetDescription() string {
	return fmt.Sprintf("mysql://%s@%s:%d/%s", c.User, c.Host, c.Port, c.Database)
}

// RedisConfig represents configuration for Redis connection
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

func (c *RedisConfig) CheckConnection() error {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", c.Host, c.Port),
		Password: c.Password,
		DB:       c.DB,
	})
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("Redis ping failed: %v", err)
	}
	return nil
}

func (c *RedisConfig) GetDescription() string {
	return fmt.Sprintf("redis://%s:%d/%d", c.Host, c.Port, c.DB)
}

// PubSubConfig represents configuration for Google Cloud Pub/Sub connection
type PubSubConfig struct {
	ProjectID       string
	CredentialsJSON string
	TopicID         string
}

func (c *PubSubConfig) CheckConnection() error {
	ctx := context.Background()

	// 创建 Pub/Sub 客户端
	var client *pubsub.Client
	var err error

	if c.CredentialsJSON != "" {
		// 使用 JSON 凭证
		client, err = pubsub.NewClient(ctx, c.ProjectID, option.WithCredentialsJSON([]byte(c.CredentialsJSON)))
	} else {
		// 使用默认凭证
		client, err = pubsub.NewClient(ctx, c.ProjectID)
	}
	if err != nil {
		return fmt.Errorf("failed to create Pub/Sub client: %v", err)
	}
	defer client.Close()

	// 检查主题是否存在
	topic := client.Topic(c.TopicID)
	exists, err := topic.Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check topic existence: %v", err)
	}
	if !exists {
		return fmt.Errorf("topic %s does not exist", c.TopicID)
	}

	return nil
}

func (c *PubSubConfig) GetDescription() string {
	return fmt.Sprintf("pubsub://%s/topics/%s", c.ProjectID, c.TopicID)
}

type ConnectionConfig struct {
	Type            string
	IsChecked       bool
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	URL             string
	Method          string
	Timeout         string
	DB              int
	ProjectID       string
	CredentialsJSON string
	TopicID         string
}

func parseConfig(configPath string) (map[string]ConnectionConfig, error) {
	v := viper.New()
	v.SetConfigFile(configPath)
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	connections := make(map[string]ConnectionConfig)
	connKeys := v.GetStringMap("check.connections")

	for name := range connKeys {
		prefix := fmt.Sprintf("check.connections.%s", name)
		config := ConnectionConfig{
			Type:            v.GetString(prefix + ".type"),
			IsChecked:       v.GetBool(prefix + ".is_checked"),
			Host:            v.GetString(prefix + ".host"),
			Port:            v.GetInt(prefix + ".port"),
			User:            v.GetString(prefix + ".user"),
			Password:        v.GetString(prefix + ".password"),
			Database:        v.GetString(prefix + ".database"),
			URL:             v.GetString(prefix + ".url"),
			Method:          v.GetString(prefix + ".method"),
			Timeout:         v.GetString(prefix + ".timeout"),
			DB:              v.GetInt(prefix + ".db"),
			ProjectID:       v.GetString(prefix + ".project_id"),
			CredentialsJSON: v.GetString(prefix + ".credentials_json"),
			TopicID:         v.GetString(prefix + ".topic_id"),
		}
		connections[name] = config
	}

	return connections, nil
}

func createResource(name string, config ConnectionConfig) (*Resource, error) {
	var resource Resource
	resource.Type = config.Type
	resource.IsChecked = config.IsChecked

	switch config.Type {
	case "mysql":
		resource.Config = &MySQLConfig{
			Host:     config.Host,
			Port:     config.Port,
			User:     config.User,
			Password: config.Password,
			Database: config.Database,
		}
	case "http":
		timeout, err := time.ParseDuration(config.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout format: %v", err)
		}
		resource.Config = &HTTPConfig{
			URL:     config.URL,
			Method:  config.Method,
			Timeout: timeout,
		}
	case "redis":
		resource.Config = &RedisConfig{
			Host:     config.Host,
			Port:     config.Port,
			Password: config.Password,
			DB:       config.DB,
		}
	case "pubsub":
		resource.Config = &PubSubConfig{
			ProjectID:       config.ProjectID,
			CredentialsJSON: config.CredentialsJSON,
			TopicID:         config.TopicID,
		}
	default:
		return nil, fmt.Errorf("unsupported resource type: %s", config.Type)
	}

	return &resource, nil
}

func newConnection() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connection",
		Short: "资源连通check",
		RunE: func(cmd *cobra.Command, args []string) error {
			// 解析配置文件
			connections, err := parseConfig("cmd/default.yaml")
			if err != nil {
				return err
			}

			// 创建表格
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.AppendHeader(table.Row{"Name", "Type", "Description", "Status"})
			t.SetStyle(table.StyleLight)

			// 检查每个连接
			for name, connConfig := range connections {
				resource, err := createResource(name, connConfig)
				if err != nil {
					t.AppendRow(table.Row{
						name,
						connConfig.Type,
						"",
						text.FgRed.Sprintf("Error: %v", err),
					})
					continue
				}

				// 如果 IsChecked 为 false，跳过检查
				if !resource.IsChecked {
					t.AppendRow(table.Row{
						name,
						connConfig.Type,
						resource.Config.GetDescription(),
						text.FgYellow.Sprint("Skipped"),
					})
					continue
				}

				err = resource.Config.CheckConnection()
				if err != nil {
					t.AppendRow(table.Row{
						name,
						connConfig.Type,
						resource.Config.GetDescription(),
						text.FgRed.Sprintf("Failed: %v", err),
					})
				} else {
					t.AppendRow(table.Row{
						name,
						connConfig.Type,
						resource.Config.GetDescription(),
						text.FgGreen.Sprint("Connected"),
					})
				}
			}

			t.Render()
			return nil
		},
	}

	return cmd
}
