package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
	_ "modernc.org/sqlite"
)

type Record struct { Account, Item, Name, Campus, Phase, Updated string }
type Journal struct { db *sql.DB; root string }

func accountKey(username string) string {
	s := sha256.Sum256([]byte(username))
	return hex.EncodeToString(s[:])
}

func lockInstance(root string) (*os.File, error) {
	return acquireInstance(root,false)
}

func acquireInstance(root string, uninstalling bool) (*os.File, error) {
	if err := os.MkdirAll(root, 0700); err != nil { return nil, err }
	check:=func()error{if uninstalling{return nil};if _,err:=os.Stat(filepath.Join(root,"uninstalling"));err==nil{return fmt.Errorf("本项目正在卸载，禁止启动新操作")}else if !os.IsNotExist(err){return err};return nil}
	if err:=check();err!=nil{return nil,err}
	f, err := os.OpenFile(filepath.Join(root, "process.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil { return nil, err }
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{}); err != nil {
		f.Close()
		return nil, fmt.Errorf("本项目已有程序运行，无法取得单实例锁")
	}
	if err:=check();err!=nil{f.Close();return nil,err}
	return f, nil
}

func openJournal(root string) (*Journal, error) {
	if err := os.MkdirAll(root, 0700); err != nil { return nil, err }
	db, err := sql.Open("sqlite", filepath.Join(root, "state.sqlite3"))
	if err != nil { return nil, err }
	db.SetMaxOpenConns(1)
	for _, query := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", `CREATE TABLE IF NOT EXISTS operations (account TEXT NOT NULL, item TEXT NOT NULL, name TEXT NOT NULL, campus TEXT NOT NULL, phase TEXT NOT NULL, updated TEXT NOT NULL, PRIMARY KEY(account,item))`} {
		if _, err := db.Exec(query); err != nil { db.Close(); return nil, err }
	}
	return &Journal{db, root}, nil
}

func (j *Journal) event(name string, fields map[string]any) error {
	if fields == nil { fields = map[string]any{} }
	fields["event"], fields["time"] = name, time.Now().UTC().Format(time.RFC3339Nano)
	b, err := json.Marshal(fields)
	if err != nil { return err }
	f, err := os.OpenFile(filepath.Join(j.root, "activity.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil { return err }
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

func (j *Journal) record(username string, t Target, phase string) error {
	allowed := map[string]bool{"enter_pending": true, "reserved": true, "save_pending": true, "complete": true, "full": true, "not_entered": true}
	if !allowed[phase] { return fmt.Errorf("未知报名阶段") }
	_, err := j.db.Exec(`INSERT INTO operations VALUES (?,?,?,?,?,?) ON CONFLICT(account,item) DO UPDATE SET name=excluded.name,campus=excluded.campus,phase=excluded.phase,updated=excluded.updated`, accountKey(username), t.ID, t.Name, t.Campus, phase, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil { return err }
	return j.event("报名阶段", map[string]any{"item": t.ID, "phase": phase})
}

func (j *Journal) rows() ([]Record, error) {
	r, err := j.db.Query("SELECT account,item,name,campus,phase,updated FROM operations ORDER BY updated DESC")
	if err != nil { return nil, err }
	defer r.Close()
	var result []Record
	for r.Next() {
		var entry Record
		if err := r.Scan(&entry.Account, &entry.Item, &entry.Name, &entry.Campus, &entry.Phase, &entry.Updated); err != nil { return nil, err }
		result = append(result, entry)
	}
	return result, r.Err()
}

func (j *Journal) pending(username string) ([]Record, error) {
	rows, err := j.rows()
	if err != nil { return nil, err }
	var result []Record
	for _, r := range rows {
		if r.Phase == "enter_pending" || r.Phase == "reserved" || r.Phase == "save_pending" {
			if r.Account != accountKey(username) { return nil, fmt.Errorf("存在其他账户的未完成提交，请切换账户核查") }
			result = append(result, r)
		}
	}
	if len(result) > 1 { return nil, fmt.Errorf("存在多场未完成提交，禁止继续报名") }
	return result, nil
}
