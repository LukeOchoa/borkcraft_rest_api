package main

import (
	"borkcraft_rest_api/errorc"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	//"golang.org/x/exp/maps"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	uuid "github.com/satori/go.uuid"
)

//	host     = "borkcraft-info.cc9gxx4itgmz.us-west-1.rds.amazonaws.com"
//	userx 	 = "borker"
//	password = "3CgE4wgUdhTA$#"

//	const (
//		host     = "localhost"
//		port     = 5432
//		userx    = "luke"
//		password = "free144"
//		dbname   = "breaker"
//	)

const (
	host     = "localhost"
	port     = 5432
	userx    = "luke"
	password = "free144"
	dbname   = "breaker"
)

func create_DB_Connection() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s "+
		"sslmode=disable",
		host, port, userx, password, dbname)
	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

type ID struct {
	number int
	text   string
}

func newID() ID {
	return ID{number: 0, text: ""}
}

type SessionKey struct {
	Id         string
	SessionID  string //uuid.UUID
	LastActive string
	Expiration string
	Username   string
}

func getValidID(dbName string) (ID, error) {
	db, err := create_DB_Connection()
	if err != nil {
		return newID(), err
	}
	defer db.Close()
	// sql string to get all primary key ids
	sql := fmt.Sprintf(`
                SELECT id From %s;`, dbName)

	var idKeys []int
	rows, err := db.Query(sql)
	if err != nil {
		return newID(), err
	}
	for rows.Next() {
		var id int
		err = rows.Scan(&id)
		if err != nil {
			panic(err)
		}
		idKeys = append(idKeys, id)
	}

	err = rows.Err()
	if err != nil {
		return newID(), err
	}

	var booly bool = false
	var lowestNewValue int = 0
	for range idKeys {
		for y := range idKeys {
			if lowestNewValue == idKeys[y] {
				booly = true
				break
			}
		}
		if booly {
			lowestNewValue = lowestNewValue + 1
		}
		booly = false
	}

	var id = ID{
		number: lowestNewValue,
		text:   strconv.Itoa(lowestNewValue),
	}

	return id, nil
}

func maker(amount int, to_make string) string {
	var make string
	for i := 0; i <= amount; i++ {
		make = make + to_make
	}

	return make
}
func tabber(amount int) string {
	return maker(amount, "\t")
}
func nliner(amount int) string {
	return maker(amount, "\n")
}

func checkIfExists(table, column, value string) (bool, error) {
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("db connection failed, ", err)
		return false, err
	}
	defer db.Close()

	sql := fmt.Sprintf(
		`SELECT exists (SELECT 1 FROM %s WHERE %s='%s');`, table, column, value)
	var available bool
	err = db.QueryRow(sql).Scan(&available)
	if err != nil {
		fmt.Println("db sql failed")
		return false, err
	}

	return available, nil
}

func selectFromDB(column, table, condition, where_condition string) (string, error) {
	db, err := create_DB_Connection()
	if err != nil {
		return "", err
	}
	defer db.Close()

	sql := fmt.Sprintf(`SELECT %s FROM %s WHERE %s='%s'`, column, table, condition, where_condition)
	var database_value string
	err = db.QueryRow(sql).Scan(&database_value)
	if err != nil {
		return "", err
	}

	return database_value, nil
}

func delete_session(username string) error {
	// if session is expired, renew it
	sql := fmt.Sprintf(
		`DELETE FROM native_user_keys WHERE username='%s';`, username)
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("db connect failed x")
		return err
	}
	defer db.Close()
	_, err = db.Exec(sql)
	if err != nil {
		fmt.Println("failed to execute sql @ delete session")
		return err
	}

	return nil
}

func createSessionKey(username string) (string, error) {
	id, err := getValidID("native_user_keys")
	if err != nil {
		fmt.Println("failed to get valid id")
		return "", err
	}

	sk := SessionKey{
		Id:         id.text,
		SessionID:  uuid.NewV4().String(),
		LastActive: time.Now().Format(RFC3339),
		Expiration: time.Now().Add(session_length()).Format(RFC3339),
		Username:   username,
	}

	columns := "id, sessionid, lastactive, expiration, username"
	values := fmt.Sprintf(`'%s', '%s', '%s', '%s', '%s'`,
		sk.Id, sk.SessionID, sk.LastActive, sk.Expiration, sk.Username)
	sql := fmt.Sprintf(
		`INSERT INTO %s(%s) VALUES(%s);`, "native_user_keys", columns, values)

	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("createSessionKey db connect fail")
		return "", err
	}
	defer db.Close()
	_, err = db.Exec(sql)
	if err != nil {
		fmt.Println("createSessionKey db exec failed")
		fmt.Println(sql)
		return "", err
	}

	return sk.SessionID, nil
}

func session_time_to_map(theTime time.Time) map[string]string {
	second := strconv.Itoa(theTime.Second())
	minute := strconv.Itoa(theTime.Minute())
	hour := strconv.Itoa(theTime.Hour())

	sessionTime := map[string]string{
		"second": second,
		"minute": minute,
		"hour":   hour,
	}

	return sessionTime
}

// func count_occurance_in_db(table, column, group_by, where, where_con string) (int, error) {
type Occur struct {
	table     string
	column    string
	group_by  string
	where     string
	where_con string
}

func (o *Occur) count_occurance_in_db() (int, error) {
	// sql := `SELECT '%s', count('%s') FROM '%s' GROUP BY '%s'`, column, column, table, group_by)

	select_from := fmt.Sprintf(
		`SELECT '%s', count('%s') FROM %s`, o.column, o.column, o.table)

	group_by := fmt.Sprintf(" GROUP BY %s", o.group_by)

	var where string
	if where != "" {
		where = fmt.Sprintf(" WHERE %s='%s'", o.where, o.where_con)
	}

	sql := select_from + where + group_by + ";"
	fmt.Println(sql)

	var waste string
	var count int
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("failed to connect to database")
		return 0, err
	}

	err = db.QueryRow(sql).Scan(&waste, &count)
	if err != nil {
		fmt.Println("error: ", err)
		return 0, err
	}

	return count, nil
}

func (nether_portal *NetherPortal) update_nether_portal_text_in_db() errorc.ErrorComplex {

	// Define Variables
	var table = "netherportals"
	var columns = []string{"id", "xcord_overworld", "ycord_overworld", "zcord_overworld", "xcord_nether", "ycord_nether", "zcord_nether", "local_overworld", "owner_overworld", "notes_overworld", "overworld_true_name", "local_nether", "owner_nether", "notes_nether", "nether_true_name", "username"}
	var values = nether_portal.to_slice_of_string()

	// Create SQL statements
	update := fmt.Sprintf(`UPDATE %s `, table)
	set := "SET "
	where := fmt.Sprintf("WHERE id = %s;", strconv.Itoa(nether_portal.Id))

	// Formating
	for i := 0; i < 16; i++ { // format together all "Set column1 = value1" data from (columns) and (values)
		if i < 7 { // integers are not wrapped
			set = set + fmt.Sprintf("%s = %s, ", columns[i], values[i])
		} else {
			if i == 15 { // last sprintf shouldn't have a comma(,)
				set = set + fmt.Sprintf("%s = '%s' ", columns[i], values[i])
			} else {
				set = set + fmt.Sprintf("%s = '%s', ", columns[i], values[i])
			}
		}
	}

	// Open/Close Connection
	db, err := create_DB_Connection()
	if err != nil {
		return errorc.New(http.StatusInternalServerError, "", err)
	}
	defer db.Close()

	// Execute SQL
	_, err = db.Exec(update + set + where)
	if err != nil {
		return errorc.New(http.StatusBadRequest, "", err)
	}

	return errorc.Nil()
}

func (nether_portal *NetherPortal) scan_rows_to_nether_portal(rows *sql.Rows) error {
	err := rows.Scan(
		&nether_portal.Id,
		&nether_portal.OverWorld.Xcord,
		&nether_portal.OverWorld.Ycord,
		&nether_portal.OverWorld.Zcord,

		&nether_portal.Nether.Xcord,
		&nether_portal.Nether.Ycord,
		&nether_portal.Nether.Zcord,

		&nether_portal.OverWorld.Locale,
		&nether_portal.OverWorld.Owner,
		&nether_portal.OverWorld.Notes,
		&nether_portal.OverWorld.True_Name,

		&nether_portal.Nether.Locale,
		&nether_portal.Nether.Owner,
		&nether_portal.Nether.Notes,
		&nether_portal.Nether.True_Name,

		&nether_portal.Username,
	)

	if err != nil {
		return err
	}

	return nil
}

func (nether_portal *NetherPortal) insert_nether_portal_text_to_db() error {

	var table = "netherportals"
	var columns = []string{"id", "xcord_overworld", "ycord_overworld", "zcord_overworld", "xcord_nether", "ycord_nether", "zcord_nether", "local_overworld", "owner_overworld", "notes_overworld", "overworld_true_name", "local_nether", "owner_nether", "notes_nether", "nether_true_name", "username"}
	var values = nether_portal.to_formated_string()

	sql := fmt.Sprintf(
		`INSERT INTO %s(%s) VALUES(%s);`, table, columns, values)
	db, err := create_DB_Connection()
	if err != nil {
		return err
	}

	_, err = db.Exec(sql)
	if err != nil {
		return err
	}

	return nil
}

func rows_in_a_table(table string) (int, error) {
	db, err := create_DB_Connection()
	if err != nil {
		panic(err)
	}

	sql := fmt.Sprintf("Select count(*) AS exact_count FROM public.%s;", table)
	var number_of_rows_in_table int
	err = db.QueryRow(sql).Scan(&number_of_rows_in_table)
	if err != nil {
		return 0, err
	}

	return number_of_rows_in_table, nil
}

func getSessionTimeToMap(theTime time.Time) map[string]string {
	second := strconv.Itoa(theTime.Second())
	minute := strconv.Itoa(theTime.Minute())
	hour := strconv.Itoa(theTime.Hour())

	sessionTime := map[string]string{
		"second": second,
		"minute": minute,
		"hour":   hour,
	}

	return sessionTime
}

func unmarshal_readCloser[T io.ReadCloser, S any](reader T) (S, error) {
	var s S

	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(body, &s)
	if err != nil {
		return s, err
	}

	return s, nil
}

func rune_to_str(ch byte) string {
	return fmt.Sprintf("%c", ch)
}
func delChar(x string, index int) string {
	s := []rune(x)
	s = append(s[0:index], s[index+1:]...)

	return string(s)
}
func remove_comma_from_end(str string) string {
	for i := len(str) - 1; i >= 0; i-- {
		if rune_to_str(str[i]) == "," {
			str = delChar(str, i)
			break
		}
	}
	return str
}
func (nether_portal *NetherPortal) update_nether_portal_text_in_db2() errorc.ErrorComplex {
	fmt.Println("Ping")

	// Define Variables
	var table = "netherportals"

	// Create SQL statements // Search by nether_true_name // Change this to be a switch on true_name nether or ow
	update := fmt.Sprintf(`UPDATE %s `, table)
	set := "SET "
	where := fmt.Sprintf("WHERE id = %d;", nether_portal.Id)

	// New Formating

	// Create all SET information
	all_sets := nether_portal.OverWorld.sql_update_sets("_overworld")
	fmt.Print("\n\n\n\nOVERWORLD?\n", all_sets)

	nether := nether_portal.Nether.sql_update_sets("_nether")
	fmt.Print("\n\n\n\nNether?\n", nether)

	//maps.Copy(all_sets, nether)
	all_sets_list := make([]string, 0)
	for _, value := range all_sets {
		push(&all_sets_list, value)
	}
	for _, value := range nether {
		push(&all_sets_list, value)
	}

	//fmt.Printf("%s%s%s", nliner(3), all_sets, nliner(3))
	for _, values := range all_sets_list {
		set = set + values + ", "
	}
	set = remove_comma_from_end(set) + " "

	// Open/Close Connection
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("db con err")
		return errorc.New(http.StatusInternalServerError, "", err)
	}
	defer db.Close()
	// Execute SQL
	changes, err := db.Exec(update + set + where)
	if err != nil {
		fmt.Print("\nSQL FAILED:\n", update+set+where)
		return errorc.New(http.StatusBadRequest, "", err)
	}
	change, err := changes.RowsAffected()
	fmt.Printf("\nAFFECTED ROWS: %d\n", change)
	if change == 0 {
		if err != nil {
			panic(err)
		}
		return errorc.New(http.StatusBadRequest, "Could not update row...", errors.New("yes"))
	}

	return errorc.Nil()
}
