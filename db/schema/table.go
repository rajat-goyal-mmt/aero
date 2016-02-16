package schema

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/thejackrabbit/aero/str"
	"github.com/tolexo/aero/panik"
)

type table struct {
	model   interface{}
	name    string
	history string
}

func NewTable(model interface{}) *table {

	typ := reflect.TypeOf(model)
	panik.If(typ.Elem() == nil, "model must be an address")

	name := str.SnakeCase(typ.Elem().Name())

	tbl := table{
		model:   model,
		name:    name,
		history: name + "_history",
	}
	return &tbl
}

func (t *table) isHistory() bool {
	return strings.HasSuffix(t.name, "_history")
}

func (t *table) exists() bool {
	sql := fmt.Sprintf("show tables like '%s'", t.name)
	return sqlHasRows(sql)
}

func (t *table) historyExists() bool {
	sql := fmt.Sprintf("show tables like '%s'", t.history)
	return sqlHasRows(sql)
}

func (t *table) drop() {
	sql := fmt.Sprintf("drop table %s", t.name)
	sqlExec(sql)
}

func (t *table) dropHistory() {
	sql := fmt.Sprintf("drop table %s", t.history)
	sqlExec(sql)
}

func (t *table) fields() []field {
	var fields []field
	sql := fmt.Sprintf("desc %s", t.name)
	err := Dbo.Raw(sql).Find(&fields).Error
	panik.On(err)
	return fields
}

func (t *table) field(name string) *field {
	flds := t.fields()
	for _, f := range flds {
		if f.Field == name {
			return &f
		}
	}
	return nil
}

func (t *table) autoIncrField() *field {
	flds := t.fields()
	for _, f := range flds {
		if strings.Contains(f.Extra, "auto_increment") {
			return &f
		}
	}
	return nil
}

func (t *table) primaryKeys() []field {
	pkeys := []field{}

	flds := t.fields()
	for _, f := range flds {
		if strings.Contains(f.Key, "PRI") {
			pkeys = append(pkeys, f)
		}
	}
	return pkeys
}

func (t *table) setupHistoryTable1() {

	// builds the history table manually

	// create alike
	sql := fmt.Sprintf("create table %s like %s;", t.history, t.name)
	sqlExec(sql)

	// add action and actioned_at (at the end)
	sql = fmt.Sprintf("alter table %s add column action varchar(6) not null default 'insert' first, add column actioned_at TIMESTAMP default current_timestamp after action", t.history)
	sqlExec(sql)

	// remove auto_incr
	autoInc := t.autoIncrField()
	if autoInc != nil {
		noAuto := strings.Replace(autoInc.info(), "auto_increment", "", -1)
		sql = fmt.Sprintf("ALTER TABLE %s MODIFY %s", t.history, noAuto)
		sqlExec(sql)
	}

	// drop primary key
	sql = fmt.Sprintf("alter table %s drop primary key", t.history)
	sqlExec(sql)

	t.setupHistoryTriggers()
}

func (t *table) readyHistoryTable() {

	if !strings.HasSuffix(t.name, "_history") {
		return
	}

	// remove auto_incr
	autoInc := t.autoIncrField()
	if autoInc != nil {
		noAuto := strings.Replace(autoInc.info(), "auto_increment", "", -1)
		sql := fmt.Sprintf("ALTER TABLE %s MODIFY %s", t.name, noAuto)
		sqlExec(sql)
	}

	// drop primary key
	pkeys := t.primaryKeys()
	if len(pkeys) > 0 {
		sql := fmt.Sprintf("alter table %s drop primary key", t.name)
		sqlExec(sql)
	}
}

func (t *table) setupHistoryTriggers() {
	// drop old triggers
	sqlExec(fmt.Sprintf("drop trigger if exists %s_insert_history", t.name))
	sqlExec(fmt.Sprintf("drop trigger if exists %s_update_history", t.name))
	sqlExec(fmt.Sprintf("drop trigger if exists %s_delete_history", t.name))
	// and create new triggers
	sql := fmt.Sprintf(`CREATE TRIGGER %s_insert_history AFTER INSERT ON %s FOR EACH ROW
        INSERT INTO %s SELECT 'insert',null, src.* 
        FROM %s as src WHERE src.id = NEW.id;`, t.name, t.name, t.history, t.name)
	sqlExec(sql)
	sql = fmt.Sprintf(`CREATE TRIGGER %s_update_history AFTER UPDATE ON %s FOR EACH ROW
    	INSERT INTO %s SELECT 'update',null, src.*
        FROM %s as src WHERE src.id = NEW.id;`, t.name, t.name, t.history, t.name)
	sqlExec(sql)
	sql = fmt.Sprintf(`CREATE TRIGGER %s_delete_history BEFORE DELETE ON %s FOR EACH ROW
        INSERT INTO %s SELECT 'delete',null, src.*
        FROM %s as src WHERE src.id = OLD.id;`, t.name, t.name, t.history, t.name)
	sqlExec(sql)
}
