package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"os"
	"strings"
)

// 用户输入配置
var (
	db_host         string // 数据库地址
	db_port         string // 数据库端口
	db_name         string // 数据库名称
	db_account      string // 数据库账号
	db_pwd          string // 数据库密码
	path            string // 结构体保存路径
	filename_format string // 文件名格式 0-表名同步 1-驼峰
	tables          string // 表名称
)

// 表类型
type Dbtabels struct {
	Name string `json:"name"`
}

// 数据字段类型
type Column struct {
	ColumnName    string         `json:"columnName"`
	DataType      string         `json:"dataType"`
	ColumnComment string         `json:"columnComment"`
	Columnkey     string         `json:"columnKey"`
	Extra         string         `json:"extra"`
	ColumnType    string         `json:"columnType"`
	ColumnDefault sql.NullString `json:"columnDefault"`
}

func init() {
	flag.StringVar(&db_account, "a", "root", "# Database account")
	flag.StringVar(&db_pwd, "p", "123123", "# Database password")
	flag.StringVar(&db_host, "h", "127.0.0.1", "# Database host")
	flag.StringVar(&db_port, "P", "3306", "# Database port")
	flag.StringVar(&db_name, "d", "", "# Database name")
	flag.StringVar(&tables, "t", "", "# Table name formats such as - t user, rule, config; \"all\" means all tables ")
	flag.StringVar(&path, "path", "", "# Structure preservation path")
	flag.StringVar(&filename_format, "fm", "0", "# Generated file name format 0-tablename 1-hump format tablename")
	flag.Parse()
}

func main() {
	// 默认空的值返回usage
	if db_name == "" || path == "" || tables == "" {
		flag.Usage()
		return
	}
	tables = convtables(tables) // 转换

	fmt.Println("Database:", db_name)
	fmt.Println("Database account:", db_account)
	fmt.Println("Database password:", db_pwd)
	fmt.Println("Structure preservation path:", path)
	fmt.Println("Tables:", tables)
	fmt.Println("Filename format:", filename_format)
	fmt.Println("Gen gorm struct start...")

	// 检查生成路径
	if err := checkpath(path); err != nil {
		fmt.Println("path error: ", err)
		return
	}

	// 连接db
	dataSourceName := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", db_account, db_pwd, db_host, db_port, db_name)
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		fmt.Println("mysql connect err:", err)
		return
	}

	// 获取所有表
	var sqls = "select table_name from information_schema.tables where table_schema=?"
	if tables != "all" {
		sqls = fmt.Sprintf(sqls+" and table_name in (%s)", tables)
	}
	rows, err := db.Query(sqls, db_name)
	if err != nil {
		fmt.Println("mysql query err:", err)
		return
	}
	defer func() {
		if rows != nil {
			rows.Close() // 可以关闭掉未scan连接一直占用
		}
	}()

	table := Dbtabels{}
	for rows.Next() {
		err := rows.Scan(&table.Name)
		fmt.Println("generate table gorm struct：", table.Name)
		if err != nil {
			fmt.Printf("Scan failed,err:%v", err)
			return
		}
		// 获取单个表所有字段
		cloumn_sql := fmt.Sprintf("select column_name columnName, data_type dataType, column_comment columnComment, column_key columnKey, extra,column_default columnDefault,column_type columnType from information_schema.columns where table_name = '%s' and table_schema = (select database()) order by ordinal_position",
			table.Name)
		cloumns, err := db.Query(cloumn_sql)
		if err != nil {
			fmt.Println("mysql query select table column err:", err)
		}
		defer func() {
			if cloumns != nil {
				cloumns.Close() // 可以关闭掉未scan连接一直占用
			}
		}()
		// 文件名
		filename := trimTablePrefix(table.Name)
		// 生成的结构体名
		structName := underlineStrToHumpStrunde(filename)
		// 转驼峰
		if filename_format == "1" {
			filename = structName
		}

		import_head := ""
		struct_str := fmt.Sprintf("type %s struct { \n", structName)
		column := Column{}
		for cloumns.Next() {
			err := cloumns.Scan(&column.ColumnName, &column.DataType, &column.ColumnComment, &column.Columnkey,
				&column.Extra, &column.ColumnDefault, &column.ColumnType)
			// 类型判断
			// 开始拼接字符串
			if err != nil {
				fmt.Printf("Scan column failed,err:%v", err)
				return
			}
			struct_str += "    " + strFirstToUpper(column.ColumnName)
			// 校验类型，有符号无符号
			isunsigned := strings.Contains(column.ColumnType, "unsigned")
			switch column.DataType {
			case "tinyint":
				if isunsigned {
					struct_str += " uint8 "
				} else {
					struct_str += " int8 "
				}
			case "int":
				if isunsigned {
					struct_str += " uint32 "
				} else {
					struct_str += " int32 "
				}
			case "bigint":
				if isunsigned {
					struct_str += " uint64 "
				} else {
					struct_str += " int64 "
				}
			case "decimal":
				struct_str += " float64 "
			case "timestamp":
				fallthrough
			case "datetime":
				import_head = "import \"time\"\n\n"
				struct_str += " time.Time "
			default:
				struct_str += " string "
			}
			// 如为主键增加primary_key;
			var primaryKey string
			if column.Columnkey == "PRI" {
				primaryKey = ";primary_key"
			}
			// 如果默认值为空，也生成默认值
			if !column.ColumnDefault.Valid {
				struct_str += fmt.Sprintf("`gorm:\"column:%s;default:%v%s\"` \n", column.ColumnName, "NULL", primaryKey)
			} else if column.ColumnDefault.String != "" {
				struct_str += fmt.Sprintf("`gorm:\"column:%s;default:%v%s\"` \n", column.ColumnName,
					column.ColumnDefault.String, primaryKey)
			} else {
				struct_str += fmt.Sprintf("`gorm:\"column:%s%s\"` \n", column.ColumnName, primaryKey)
			}
		}
		struct_str += "}"
		model_head := "package model \n\n"
		tablename_impl_func_str := fmt.Sprintf("\n\nfunc (*%s) TableName() string {\n    return \"%s\"\n}", structName,
			table.Name)
		// 导出文件
		body := model_head + import_head + struct_str + tablename_impl_func_str
		file := fmt.Sprintf("%s/%s.go", path, filename)
		// 创建文件夹
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			fmt.Println("midkr path error:", err)
		}
		err = ioutil.WriteFile(file, []byte(body), 0666)
		if err != nil {
			fmt.Println("write file error:", err)
		}
	}

	fmt.Println("End  SUCCESS")
}

// 首字母大写
func strFirstToUpper(str string) string {
	var upperStr string
	vv := []rune(str) // 后文有介绍
	for i := 0; i < len(vv); i++ {
		if i == 0 {
			if vv[i] >= 97 && vv[i] <= 122 { // 后文有介绍
				vv[i] -= 32 // string的码表相差32位
				upperStr += string(vv[i])
			} else {
				fmt.Println("Not begins with lowercase letter,")
				return str
			}
		} else {
			upperStr += string(vv[i])
		}
	}
	return upperStr
}

// 判断用户输入
func checkpath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("path is not dir!")
	}
	return nil
}

// 用户输入转换
func convtables(tables string) string {
	if tables == "all" {
		return tables
	}
	table_slice := strings.Split(tables, ",")
	// 重新组装tables字符串，去掉空格，拼接引号用于后续查询
	tables = ""
	for _, v := range table_slice {
		v = strings.TrimSpace(v)
		if v != "" {
			item := fmt.Sprintf("'%s',", v)
			tables += item
		}
	}
	// 'mall_account','mall_goods'
	return strings.TrimSuffix(tables, ",")
}

// trimTablePrefix 去掉表前缀，此处简单写，直接去掉第一个下划线前的字符
func trimTablePrefix(tablename string) string {
	tslice := strings.Split(tablename, "_")
	// 去掉第一个元素
	tslice = tslice[1:]
	return strings.Join(tslice, "_") // 重组
}

// 下划线转驼峰
func underlineStrToHumpStrunde(str string) string {
	strslice := strings.Split(str, "_")
	var humpStr string
	for _, v := range strslice {
		humpStr += strFirstToUpper(v)
	}
	return humpStr
}
