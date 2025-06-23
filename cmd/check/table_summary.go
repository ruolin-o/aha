package check

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newTableSummary() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "table-summary",
		Short: "检查配置连接的数据库中对应的数据总量",
		RunE: func(cmd *cobra.Command, args []string) error {
			v := viper.New()
			v.SetConfigFile("cmd/default.yaml")
			v.SetConfigType("yaml")
			if err := v.ReadInConfig(); err != nil {
				return fmt.Errorf("failed to read config file: %v", err)
			}

			conns := v.GetStringMap("check.connections")
			for name := range conns {
				prefix := fmt.Sprintf("check.connections.%s", name)
				typeVal := v.GetString(prefix + ".type")
				if typeVal != "mysql" {
					continue
				}
				user := v.GetString(prefix + ".user")
				password := v.GetString(prefix + ".password")
				host := v.GetString(prefix + ".host")
				port := v.GetInt(prefix + ".port")
				database := v.GetString(prefix + ".database")
				dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", user, password, host, port, database)
				connect := NewConnect(dsn)
				tables := connect.GetTables()

				// 输出分组标题
				fmt.Printf("\n连接名: %s\nDSN: %s\n", name, dsn)

				tw := table.NewWriter()
				tw.SetOutputMirror(os.Stdout)
				tw.AppendHeader(table.Row{"表名称", "记录总数"})

				for _, tbl := range tables {
					count := connect.GetTableCount(tbl)
					tw.AppendRow(table.Row{tbl, count})
				}
				tw.Render()
			}
			return nil
		},
	}
	return cmd
}

type Connect struct {
	DSN string
}

func NewConnect(dsn string) *Connect {
	return &Connect{
		DSN: dsn,
	}
}

func (c *Connect) GetTables() []string {
	db, err := sql.Open("mysql", c.DSN)
	if err != nil {
		log.Printf("failed to connect to MySQL: %v", err)
		return nil
	}
	defer db.Close()

	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		log.Printf("failed to query tables: %v", err)
		return nil
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			log.Printf("failed to scan table name: %v", err)
			continue
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		log.Printf("row iteration error: %v", err)
	}
	return tables
}

func (c *Connect) GetTableCount(tableName string) int {
	db, err := sql.Open("mysql", c.DSN)
	if err != nil {
		return -1
	}
	defer db.Close()

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)
	err = db.QueryRow(query).Scan(&count)
	if err != nil {
		return -1
	}
	return count
}
