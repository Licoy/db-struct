package dbstruct

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

var types = map[string]string{
	"int":                "int",
	"integer":            "int",
	"tinyint":            "int8",
	"smallint":           "int16",
	"mediumint":          "int32",
	"bigint":             "int64",
	"int unsigned":       "int64",
	"integer unsigned":   "int64",
	"tinyint unsigned":   "int64",
	"smallint unsigned":  "int64",
	"mediumint unsigned": "int64",
	"bigint unsigned":    "int64",
	"bit":                "int64",
	"float":              "float64",
	"double":             "float64",
	"decimal":            "float64",
	"binary":             "string",
	"varbinary":          "string",
	"enum":               "string",
	"set":                "string",
	"varchar":            "string",
	"char":               "string",
	"tinytext":           "string",
	"mediumtext":         "string",
	"text":               "string",
	"longtext":           "string",
	"blob":               "string",
	"tinyblob":           "string",
	"mediumblob":         "string",
	"longblob":           "string",
	"bool":               "bool",
	"date":               "time.Time",
	"datetime":           "time.Time",
	"timestamp":          "time.Time",
	"time":               "time.Time",
}

type FmtMode uint16

const (
	FmtDefault                 FmtMode = iota //默认(和表名一致)
	FmtUnderlineToStartUpHump                 //下划线转开头大写驼峰
	FmtUnderlineToStartLowHump                //下划线开头小写驼峰
	FmtUnderline                              //下划线格式
)

type dbStruct struct {
	dsn              string   //数据库链接
	tables           []string //自定义表
	tagJson          bool     //json tag
	tagOrm           bool     //orm tag
	fieldNameFmt     FmtMode
	structNameFmt    FmtMode
	fileNameFmt      FmtMode
	genTableName     string
	genTableNameFunc bool
	modelPath        string
	singleFile       bool
	packageName      string
	tags             []*Tag
	db               *sql.DB
	err              error
}

func NewDBStruct() *dbStruct {
	return &dbStruct{}
}

func (ds *dbStruct) Dsn(v string) *dbStruct {
	ds.dsn = v
	return ds
}

func (ds *dbStruct) GenTableName(v string) *dbStruct {
	ds.genTableName = v
	return ds
}

func (ds *dbStruct) PackageName(v string) *dbStruct {
	ds.packageName = v
	return ds
}

func (ds *dbStruct) GenTableNameFunc(v bool) *dbStruct {
	ds.genTableNameFunc = v
	return ds
}

func (ds *dbStruct) SingleFile(v bool) *dbStruct {
	ds.singleFile = v
	return ds
}

func (ds *dbStruct) FileNameFmt(v FmtMode) *dbStruct {
	ds.fileNameFmt = v
	return ds
}

func (ds *dbStruct) FieldNameFmt(v FmtMode) *dbStruct {
	ds.fieldNameFmt = v
	return ds
}

func (ds *dbStruct) StructNameFmt(v FmtMode) *dbStruct {
	ds.structNameFmt = v
	return ds
}

func (ds *dbStruct) AppendTable(v string) *dbStruct {
	if ds.tables == nil {
		ds.tables = make([]string, 0, 10)
	}
	ds.tables = append(ds.tables, v)
	return ds
}

func (ds *dbStruct) TagJson(v bool) *dbStruct {
	ds.tagJson = v
	return ds
}

func (ds *dbStruct) TagOrm(v bool) *dbStruct {
	ds.tagOrm = v
	return ds
}

func (ds *dbStruct) AppendTag(v *Tag) *dbStruct {
	if ds.tags == nil {
		ds.tags = make([]*Tag, 0, 10)
	}
	ds.tags = append(ds.tags, v)
	return ds
}

type Tag struct {
	TagName string
	Mode    FmtMode
}

type column struct {
	Name     string
	Type     string
	Nullable string
	Table    string
	Comment  string
}

func NewTag(tagName string, mode FmtMode) *Tag {
	return &Tag{TagName: tagName, Mode: mode}
}

func (ds *dbStruct) connectDB() {
	if ds.db == nil {
		ds.db, ds.err = sql.Open("mysql", ds.dsn)
	}
}

//生成
func (ds *dbStruct) Generate() (err error) {
	if ds.dsn == "" {
		return errors.New("DSN未配置")
	}
	ds.connectDB()
	if ds.err != nil {
		return ds.err
	}
	if ds.tagJson {
		ds.AppendTag(NewTag("json", FmtDefault))
	}
	if ds.tagOrm {
		ds.AppendTag(NewTag("orm", FmtDefault))
	}
	//tables := make(map[string][]column)
	tables, err := ds.getTables()
	if err != nil {
		return
	}
	writes := make(map[string]string)
	for table, columns := range tables {
		structName, content, err := ds.genStruct(table, columns)
		if err != nil {
			log.Fatalf("%s结构生成失败：%s\n", table, err.Error())
		}
		writes[structName] = content
	}

	if ds.modelPath == "" {
		ds.modelPath, err = os.Getwd()
		if err != nil {
			return err
		}
		if ds.singleFile {
			ds.modelPath += "/model/models.go"
		} else {
			ds.modelPath += "/model"
		}
	}

	if ds.singleFile {

		dir, _ := filepath.Split(ds.modelPath)

		err = os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Println("base path create fail.")
			return err
		}

		_, err := os.Create(ds.modelPath)
		if err != nil {
			return err
		}

		finalContent := bytes.Buffer{}
		finalContent.WriteString(fmt.Sprintf("package %s\n\n", ds.packageName))
		for _, content := range writes {
			finalContent.WriteString(content)
			finalContent.WriteString("\n\n\n")
		}
		err = ds.writeStruct(ds.modelPath, finalContent.String())
		if err != nil {
			log.Fatalf("write struct fail(%s) : %s ", ds.modelPath, err.Error())
			return err
		}

		cmd := exec.Command("gofmt", "-w", ds.modelPath)
		_ = cmd.Run()

	} else {

		err = os.MkdirAll(ds.modelPath, os.ModePerm)
		if err != nil {
			log.Println("base path create fail.")
			return err
		}

		for name, content := range writes {
			filename := ds.getFormatName(name, ds.fileNameFmt)
			filename = fmt.Sprintf("%s/%s.go", ds.modelPath, filename)
			err := ds.writeStruct(filename, content)
			if err != nil {
				log.Fatalf("write struct fail(%s) : %s ", filename, err.Error())
				continue
			}
			cmd := exec.Command("gofmt", "-w", filename)
			_ = cmd.Run()
		}

	}

	return
}

func (ds *dbStruct) getFormatName(s string, m FmtMode) (res string) {
	switch m {
	case FmtUnderlineToStartUpHump:
		{
			split := strings.Split(s, "_")
			res = ""
			for _, v := range split {
				res += strings.ToUpper(v[0:1]) + v[1:]
			}
		}
	case FmtUnderlineToStartLowHump:
		{
			split := strings.Split(s, "_")
			res = ""
			for i, v := range split {
				if i == 0 {
					res += strings.ToLower(v[0:1])
				} else {
					res += strings.ToUpper(v[0:1])
				}
				res += v[1:]
			}
		}
	case FmtUnderline:
		{
			b := bytes.Buffer{}
			for i, v := range s {
				if unicode.IsUpper(v) {
					if i != 0 {
						b.WriteString("_")
					}
					b.WriteString(string(unicode.ToLower(v)))
				} else {
					b.WriteString(string(v))
				}
			}
			res = b.String()
		}
	case FmtDefault:
		res = s
	}
	return
}

func (ds *dbStruct) getColumnGoType(dbType string) (res string) {
	res, has := types[dbType]
	if !has {
		res = "string"
		return
	}
	return
}

func (ds *dbStruct) genStruct(table string, columns []column) (structName string, content string, err error) {
	buffer := bytes.Buffer{}
	structName = ds.getFormatName(table, ds.structNameFmt)
	if !ds.singleFile {
		buffer.WriteString(fmt.Sprintf("package %s\n\n", ds.packageName))
	}
	buffer.WriteString(fmt.Sprintf("type %s struct {\n", structName))
	for _, column := range columns {
		columnName := ds.getFormatName(column.Name, ds.fieldNameFmt)
		goType := ds.getColumnGoType(column.Type)
		tagString := ""
		if ds.tags != nil && len(ds.tags) > 0 {
			tagString = "`"
			for _, tag := range ds.tags {
				tagString += fmt.Sprintf("%s:\"%s\" ", tag.TagName, ds.getFormatName(column.Name, tag.Mode))
			}
			tagString += "`"
		}
		buffer.WriteString(fmt.Sprintf("%s %s %s\n", columnName, goType, tagString))
	}
	buffer.WriteString("}\n\n")
	if ds.genTableNameFunc && ds.genTableName != "" {
		buffer.WriteString(fmt.Sprintf("func (%s *%s) %s() string {\n\treturn \"%s\"\n}", strings.ToLower(structName[0:1]),
			structName, ds.genTableName, table))
	}
	content = buffer.String()
	return
}

func (ds *dbStruct) getTables() (tables map[string][]column, err error) {
	tableIn := ""
	if ds.tables != nil && len(ds.tables) > 0 {
		buff := bytes.Buffer{}
		buff.WriteString("AND TABLE_NAME IN (")
		for i, tableName := range ds.tables {
			buff.WriteString("'")
			buff.WriteString(tableName)
			buff.WriteString("'")
			if i != len(ds.tables)-1 {
				buff.WriteString(", ")
			}
		}
		buff.WriteString(")")
	}
	sqlString := fmt.Sprintf("SELECT COLUMN_NAME AS `Name`,DATA_TYPE AS `Type`,IS_NULLABLE AS `Nullable`,TABLE_NAME AS "+
		"`Table`,COLUMN_COMMENT AS `Comment` FROM information_schema.COLUMNS WHERE table_schema=DATABASE () %s ORDER BY"+
		" TABLE_NAME ASC", tableIn)
	rows, err := ds.db.Query(sqlString)
	if err != nil {
		return nil, err
	}

	defer func() {
		qerr := rows.Close()
		if qerr != nil {
			log.Fatalf("关闭数据查询结果异常：%s", qerr.Error())
		}
	}()

	tables = make(map[string][]column, 3)

	for rows.Next() {
		c := column{}
		err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Table, &c.Comment)
		if err != nil {
			return nil, err
		}
		_, has := tables[c.Table]
		if !has {
			tables[c.Table] = make([]column, 0, 3)
		}
		tables[c.Table] = append(tables[c.Table], c)
	}

	return
}

func (ds *dbStruct) writeStruct(filepath string, content string) (err error) {
	b := []byte(content)
	err = ioutil.WriteFile(filepath, b, os.ModePerm)
	if err != nil {
		return err
	}
	return nil
}