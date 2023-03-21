package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"

	"github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"

	// local
	errorc "borkcraft_rest_api/errorc"
)

const RFC3339 = "2006-01-02T15:04:05Z07:00"

type Profile struct {
	Username   string
	Password   string
	SessionKey string
}

func (profile *Profile) Decode(reader io.Reader) error {
	decoder := json.NewDecoder(reader)
	err := decoder.Decode(&profile)

	return err
}

type Logout struct {
	Username    string
	Password    string
	Session_Key string
}

type Portal struct {
	Xcord     int
	Ycord     int
	Zcord     int
	Locale    string
	Owner     string
	Notes     string
	True_Name string
}

type NetherPortal struct {
	Id        int
	Nether    Portal
	OverWorld Portal
	Username  string
}

func (netherPortal *NetherPortal) to_slice_of_string() []string {
	values := []string{
		strconv.Itoa(netherPortal.Id),

		strconv.Itoa(netherPortal.OverWorld.Xcord),
		strconv.Itoa(netherPortal.OverWorld.Ycord),
		strconv.Itoa(netherPortal.OverWorld.Zcord),
		strconv.Itoa(netherPortal.Nether.Xcord),
		strconv.Itoa(netherPortal.Nether.Ycord),
		strconv.Itoa(netherPortal.Nether.Zcord),
		netherPortal.OverWorld.Locale,
		netherPortal.OverWorld.Owner,
		netherPortal.OverWorld.Notes,
		netherPortal.OverWorld.True_Name,

		netherPortal.Nether.Locale,
		netherPortal.Nether.Owner,
		netherPortal.Nether.Notes,
		netherPortal.Nether.True_Name,

		netherPortal.Username,
	}

	return values
}

func (netherPortal *NetherPortal) to_formated_string() string {
	values := netherPortal.to_slice_of_string()

	var the_string string

	var len = len(values) - 1
	for i, v := range values {
		if len != i {
			the_string = the_string + fmt.Sprintf("'%s', ", v)
		} else {
			the_string = the_string + fmt.Sprintf("'%s'", v)
		}
	}

	return the_string
}

type ImageDetails struct {
	Id        int
	Name      string
	True_Name string
	Username  string
}

func (im *ImageDetails) Insert_in_db() errorc.ErrorComplex {
	db, err := create_DB_Connection()
	if err != nil {
		return errorc.New(http.StatusInternalServerError, "", err)
	}

	id, err := getValidID("netherportal_images")
	if err != nil {
		return errorc.New(http.StatusInternalServerError, "", err)
	}
	//im := &image_details
	sql1 := "INSERT INTO netherportal_images(id, name, true_name, username)"
	sql2 := fmt.Sprintf(`VALUES('%s', '%s', '%s', '%s');`, id.text, im.Name, im.True_Name, im.Username)
	fmt.Println(sql1 + sql2)
	_, err = db.Exec(sql1 + sql2)
	if err != nil {
		return errorc.New(http.StatusBadRequest, "", err)
	}

	return errorc.Nil()
}

func refresh_session(username string) (string, int) {
	err := delete_session(username)
	if err != nil {
		return "", http.StatusInternalServerError
	}

	session_key, err := createSessionKey(username)
	if err != nil {
		return "", http.StatusForbidden
	}

	return session_key, 0
}

func login(writer http.ResponseWriter, request *http.Request) {
	var profile Profile
	profile.Decode(request.Body)

	// Check if the given username exists
	booly, err := checkIfExists("userprofile", "username", profile.Username)
	if err != nil {
		fmt.Println("failed to get username.")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !booly {
		fmt.Println("Username is bad.")
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	// Check if the given password is correct
	storedPassword, err := selectFromDB("password", "userprofile", "username", profile.Username)
	if err != nil {
		fmt.Println("failed to get password.")
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(profile.Password))
	if err != nil {
		fmt.Println("passwords dont match")
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	// check if the user has a session
	booly, err = checkIfExists("native_user_keys", "username", profile.Username)
	if err != nil {
		fmt.Println("failed to get session...")
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	var session_key string
	if !booly {
		fmt.Println("Existsing Session")
		// If the user doesnt have a session

		// Create a session key & push it to the Database
		session_key, err = createSessionKey(profile.Username)
		if err != nil {
			fmt.Println("10")
			writer.WriteHeader(http.StatusForbidden)
			return
		}

	} else {
		// Delete the old session and just give another
		fmt.Println("Refreshed Session")
		var code int
		session_key, code = refresh_session(profile.Username)
		if code != 0 {
			writer.WriteHeader(code)
			return
		}
	}

	// Get the session time
	time_string, err := selectFromDB("expiration", "native_user_keys", "username", profile.Username)
	if err != nil {
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	// Convert it to a real "Time" object
	the_time, err := time.Parse(RFC3339, time_string)
	if err != nil {
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	// Check if the session is expired
	now, err := time.Parse(RFC3339, time.Now().Format(RFC3339))
	if err != nil {
		fmt.Println("failed to parse time")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	if now.After(the_time) {
		// if session is expired, renew it

		if delete_session(profile.Username) != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		session_key, err = createSessionKey(profile.Username)
		if err != nil {
			fmt.Println("failed to create session key @ after now.After")
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Create a object detailing the time to send to the client
	var message = map[string]interface{}{
		"key":  session_key,
		"time": session_time_to_map(the_time),
	}
	r, err := json.Marshal(message)
	if err != nil {
		fmt.Println("failed to marshal json")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println(message)
	writer.WriteHeader(http.StatusAccepted)
	writer.Write(r)
}

func logout(writer http.ResponseWriter, request *http.Request) {
	var body Logout
	decoder := json.NewDecoder(request.Body)
	err := decoder.Decode(&body)
	if err != nil {
		fmt.Println("error:", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	booly, err := checkIfExists("native_user_keys", "sessionid", body.Session_Key)
	if err != nil {
		fmt.Println("error:", err)
		writer.WriteHeader(http.StatusInternalServerError)
	}
	if booly {
		if delete_session(body.Username) != nil {
			writer.WriteHeader(http.StatusInternalServerError)
		}
		writer.WriteHeader(http.StatusAccepted)
	}
}

func add_nether_portal_text(writer http.ResponseWriter, request *http.Request) {

	var nether_portal NetherPortal
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	err = json.Unmarshal(body, &nether_portal)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var o Occur = Occur{
		table:    "netherportals",
		column:   "username",
		group_by: "username",
	}
	count, err := o.count_occurance_in_db() // count_occurance_in_db("netherportals", "username", nether_portal.Username)
	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
	}

	if count > 9 {
		fmt.Println("addNetherPortal(): -> Too many profiles already...")
		writer.WriteHeader(http.StatusForbidden)
		return
	}

	if nether_portal.insert_nether_portal_text_to_db() != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	writer.WriteHeader(http.StatusAccepted)
}

func add_nether_portal_image_details(writer http.ResponseWriter, request *http.Request) {

	// Convert response to useable object
	image_details, err := unmarshal_readCloser[io.ReadCloser, ImageDetails](request.Body)
	if err != nil {
		fmt.Println("body could not be processed: ", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Open/Check connection
	db, err := create_DB_Connection()
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Check if user has > 10 (the max amount allowed: 10) db columns
	var o Occur = Occur{
		table:     "netherportal_images",
		column:    "true_name",
		where:     "username",
		where_con: image_details.Name,
		group_by:  "true_name",
	}
	count, err := o.count_occurance_in_db()
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// If they still have room(> 10)
	if count > 9 {
		writer.WriteHeader(http.StatusForbidden)
		return
	}
	// Insert records into database
	errc := image_details.Insert_in_db()
	if errc.Err != nil {
		fmt.Print("\nAt line 329: \n", errc.Err.Error())
		writer.WriteHeader(errc.Code)
		return
	}

	writer.WriteHeader(http.StatusAccepted)
}

func save_nether_portal_text_changes(writer http.ResponseWriter, request *http.Request) {

	// Convert response from (Request) to (NetherPortal Object)

	// To []bytes from *http.Request
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		fmt.Println("failed to convert request.Body to slice of bytes\n ->: ", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var nether_portal NetherPortal

	// To NetherPortal from []bytes
	err = json.Unmarshal(body, &nether_portal)
	if err != nil {
		fmt.Println("failed to convert nether_portal from request.Body\n ->: ", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Update database
	errc := nether_portal.update_nether_portal_text_in_db()
	if errc.Err != nil {
		fmt.Println("failed to upate (nether portal text)\n ->: ", errc.Err)
		writer.WriteHeader(errc.Code)
		return
	}

	// On success
	writer.WriteHeader(http.StatusAccepted)
}

func get_nether_portals_text_information(writer http.ResponseWriter, request *http.Request) {

	// Open/Close connection
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("Database connection failed in get_nether_portals_text_information()... ->:\n", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Define Variables
	var orderby string = request.URL.Query()["offset"][0]
	var limit string = request.URL.Query()["limit"][0]

	// Format SQL Statement
	sql := fmt.Sprintf(`SELECT * FROM netherportals WHERE id > %s ORDER BY id LIMIT %s;`, orderby, limit)

	// Query Databse
	rows, err := db.Query(sql)
	if err != nil {
		fmt.Println("get_nether_portals_text_information() query failed...\n", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Create a container for all NetherPortals to be sent back
	// Create a key for hashmap
	var nether_portal NetherPortal
	nether_portals := make(map[string]NetherPortal, 5)
	hashmap_key, err := strconv.Atoi(orderby)
	if err != nil {
		fmt.Println("get_nether_portals_text_information() convertion failed...", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Reem through and assign nether portals to container
	for rows.Next() {
		err = nether_portal.scan_rows_to_nether_portal(rows)
		if err != nil {
			fmt.Println("failed to scan row in get_nether_portals_text_information()...\n", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		nether_portals[strconv.Itoa(hashmap_key)] = nether_portal
		nether_portal = NetherPortal{}
		hashmap_key++
	}

	// Convert Map[string]NetherPortals to JSON
	json_nether_portals, err := json.Marshal(nether_portals)
	if err != nil {
		fmt.Println("failed to marshal netherportals to json, get_nether_portals_text_information()...\n", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	fmt.Println("netherportal text payload:   \n", nether_portals)
	// On Success
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusAccepted)
	writer.Write(json_nether_portals)
}

func get_access_rights(writer http.ResponseWriter, request *http.Request) {

	// Format SQL
	username := request.URL.Query()["username"][0]
	sql := fmt.Sprintf(`SELECT netherportals FROM netherportal_access_rights WHERE username='%s';`, username)

	// Open/Close connection
	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("db connection failed ...\n", err)
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Query Database
	var access_rights []string
	err = db.QueryRow(sql).Scan(pq.Array((&access_rights)))
	if err != nil {
		fmt.Println("failed to get access rights...\n", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// Convert query to JSON
	var access_rights_map = map[string][]string{
		"access_rights": access_rights,
	}
	your_moms_payload, err := json.Marshal(access_rights_map)
	if err != nil {
		fmt.Println("Your mom's payload failed?")
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// On Success
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusAccepted)
	writer.Write(your_moms_payload)
}

func session_time_left(writer http.ResponseWriter, request *http.Request) {
	fmt.Println("session_time_left")

	type Key struct {
		Key string
	}

	// Convert request into Key Struct
	var key Key
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		fmt.Println("session_time_left: ->:\n", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	err = json.Unmarshal(body, &key)
	if err != nil {
		fmt.Println("session_time_left: ->:\n", err)
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	// if Key is empty, the client doesnt have a session
	var sessionTime map[string]string
	if key.Key == "" {
		fmt.Println("no key?")
		sessionTime = map[string]string{
			"second": "",
			"minute": "",
			"hour":   "",
		}
	} else {
		// if the key is not empty, grab the session from the database
		theTimeString, err := selectFromDB("expiration", "native_user_keys", "sessionid", key.Key)
		if err != nil { // on nil, it means the user session for some reason doesnt exist
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		// Parse the time string from the DB into a real Time object
		theTime, err := time.Parse(RFC3339, theTimeString)
		if err != nil { // server error if its nil
			fmt.Println("time parsing error in session_time_left()...\n", err)
			writer.WriteHeader(http.StatusInternalServerError)
		}

		// Check the difference between the Time Object from the database and the current time
		// Send the difference back the the Client
		// A negative difference or == difference means the session is invalid
		// otherwise the session is still proceeding

		// Compare last session time in database to current time
		theTimeMap := getSessionTimeToMap(theTime)
		theTimeNowMap := getSessionTimeToMap(time.Now())

		// You logged in @ 5:30

		// Your session expires @ in (your session) + (5 minutes)

		// if (time.Now()) IS GREATER THAN (your session) + (5 minutes)

		if time.Now().After(theTime.Add(session_length())) {
			delete_session_line := fmt.Sprintf("DELETE FROM native_user_keys WHERE sessionid='%s';", key.Key)
			db, err := create_DB_Connection()
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				return
			}

			_, err = db.Exec(delete_session_line)
			if err != nil {
				writer.WriteHeader(http.StatusInternalServerError)
				fmt.Printf("SESSION KEY: %s\n", key.Key)
				return
			}
		} else {
			for mapkey := range theTimeNowMap {
				left, err := strconv.Atoi(theTimeMap[mapkey])
				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}

				right, err := strconv.Atoi(theTimeNowMap[mapkey])
				if err != nil {
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				theTimeMap[mapkey] = strconv.Itoa(left - right)
			}

		}

		//
		//if theTime.Add(time.Second * 90).Aft
		//// count the difference for each second, minute, hour key in the (time map)
		//for mapkey := range theTimeNowMap {
		//	left, err := strconv.Atoi(theTimeMap[mapkey])
		//	if err != nil {
		//		writer.WriteHeader(http.StatusInternalServerError)
		//		return
		//	}

		//	right, err := strconv.Atoi(theTimeNowMap[mapkey])
		//	if err != nil {
		//		writer.WriteHeader(http.StatusInternalServerError)
		//		return
		//	}
		//	// assign the time difference to the (time map)
		//	if mapkey == "second" && (left-right) < 1 {
		//		delete_session_line := fmt.Sprintf("DELETE FROM native_user_keys WHERE sessionid='%s';", key.Key)
		//		db, err := create_DB_Connection()
		//		if err != nil {
		//			writer.WriteHeader(http.StatusInternalServerError)
		//			return
		//		}

		//		_, err = db.Exec(delete_session_line)
		//		if err != nil {
		//			writer.WriteHeader(http.StatusInternalServerError)
		//			fmt.Printf("SESSION KEY: %s\n", key.Key)
		//			return
		//		}

		//		key.Key = ""
		//		fmt.Printf("\nwe broke, Place|%s|, Left|%d| ---- Right|%d|", mapkey, left, right)
		//		break
		//	}

		//	theTimeMap[mapkey] = strconv.Itoa(left - right)
		//}
		// copy map over so we can send it to the Client
		sessionTime = theTimeMap
	}

	//// Convert SessionTime to JSON
	//theTimeJson, err := json.Marshal(sessionTime)
	//if err != nil {
	//	writer.WriteHeader(http.StatusInternalServerError)
	//	return
	//}

	// Create a session object WITH a UPDATED key (very imporant!!!)
	var theSessionObject = map[string]interface{}{
		"key":  key.Key,
		"time": sessionTime,
	}

	//fmt.Println("The Session Object\n", theSessionObject)
	// Convert to JSON
	theSessionObjectJSON, err := json.Marshal(theSessionObject)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	// On Success
	writer.WriteHeader(http.StatusAccepted)
	writer.Write(theSessionObjectJSON)
}

func basic_message(writer http.ResponseWriter, request *http.Request) {

	var msg = map[string]string{
		"message": "This is a message from EC2",
	}
	message, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Write(message)

}

func test_connection(writer http.ResponseWriter, request *http.Request) {

	var msg = map[string]string{
		"To Ryan": "Your a pp head",
	}
	message, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}

	db, err := create_DB_Connection()
	if err != nil {
		fmt.Println("failed to connect to databse")
		panic(err)
	} else {
		fmt.Println("Made it to database")
	}
	defer db.Close()

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(message)

}

func get_nether_portal_image_names(writer http.ResponseWriter, request *http.Request) {
	db, err := create_DB_Connection()
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	sql_read := fmt.Sprintf("SELECT * FROM netherportal_images WHERE true_name='%s';", request.URL.Query()["true_name"][0])
	type ImageName struct {
		Id        int
		Name      string
		True_name string
		Username  string
	}
	imageNames := make(map[int]ImageName)
	rows, err := db.Query(sql_read)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	for rows.Next() {
		var imageName ImageName
		err = rows.Scan(
			&imageName.Id,
			&imageName.Name,
			&imageName.True_name,
			&imageName.Username,
		)
		imageNames[imageName.Id] = imageName
		if err != nil {
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

	}

	someImageNames, err := json.Marshal(&imageNames)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(someImageNames)
	db.Close()
}

func nether_portals_estimated_amount(writer http.ResponseWriter, request *http.Request) {
	number_of_rows_in_table, err := rows_in_a_table("netherportals")

	payload := map[string]int{
		"count": number_of_rows_in_table,
	}

	json_payload, err := json.Marshal(payload)
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.Write(json_payload)

}

func delete_image_from_client(writer http.ResponseWriter, request *http.Request) {

	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	var imageDetails ImageDetails
	err = json.Unmarshal(body, &imageDetails)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}

	db, err := create_DB_Connection()
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}

	sql := fmt.Sprintf(`
		DELETE FROM netherportal_images WHERE name='%s';
	`, imageDetails.Name)
	_, err = db.Exec(sql)
	if err != nil {
		writer.WriteHeader(http.StatusBadRequest)
		return
	}
	
	fmt.Printf("The image: -> |%s| was successfully deleted", imageDetails.Name)
	writer.WriteHeader(http.StatusAccepted)
}

func doNothing(w http.ResponseWriter, r *http.Request) {}
func main() {
	var OpeningMessage = "Server running on localhost:8334..."
	fmt.Println(OpeningMessage)

	http.HandleFunc("/favicon.ico", doNothing)
	http.HandleFunc("/", test_connection)
	http.HandleFunc("/login", login)
	http.HandleFunc("/logout", logout)

	http.HandleFunc("/addnetherportaltext", add_nether_portal_text)
	http.HandleFunc("/addnetherportalimagedetails", add_nether_portal_image_details)

	http.HandleFunc("/savenetherportaltextchanges", save_nether_portal_text_changes) // Save nether portal text changes
	http.HandleFunc("/getnetherportalstextinformation", get_nether_portals_text_information)
	http.HandleFunc("/getnetherportalimagenames", get_nether_portal_image_names)

	http.HandleFunc("/deleteimagefromclient", delete_image_from_client)

	http.HandleFunc("/getaccessrights", get_access_rights)
	http.HandleFunc("/sessiontimeleft", session_time_left)

	http.HandleFunc("/netherportalsestimatedamount", nether_portals_estimated_amount)

	http.ListenAndServe(":8334", nil)
}
