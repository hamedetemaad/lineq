package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"

	b64 "encoding/base64"
	"encoding/json"
)

var peers []*Client
var sortedEntries map[string][]string
var tables = make(map[string]Table)

type Config struct {
	TCP_HOST                string           `json:"tcp_host" default:"localhost"`
	WEB_HOST                string           `json:"web_host" default:"localhost"`
	TCP_PORT                string           `json:"tcp_port" default:"11111"`
	WEB_PORT                string           `json:"web_port" default:"8060"`
	TARGET_PORT             string           `json:"target_port" default:"80"`
	SERVICE_MODE            string           `json:"service_mode" default:"agg"`
	VWR_ROOM_TABLE          string           `json:"vwr_room_table" default:"room"`
	VWR_INACTIVITY_DURATION int              `json:"vwr_inactivity_duration"`
	VWR_ROUTES              map[string]Route `json:"routes"`
}

type Route struct {
	TOTAL_ACTIVE_USERS int    `json:"vwr_active_users"`
	PATH               string `json:"path"`
}

func setDefaults(config *Config) {
	valueType := reflect.ValueOf(config)
	valueTypeKind := valueType.Kind()

	if valueTypeKind != reflect.Ptr || valueType.Elem().Kind() != reflect.Struct {
		fmt.Println("Input must be a pointer to a struct")
		return
	}

	valueType = valueType.Elem()
	valueTypeType := valueType.Type()

	for i := 0; i < valueType.NumField(); i++ {
		field := valueType.Field(i)
		fieldType := valueTypeType.Field(i)
		fmt.Println(fieldType)

		if field.IsZero() {
			defaultValueTag := fieldType.Tag.Get("default")

			if defaultValueTag != "" {
				switch field.Kind() {
				case reflect.Int:
					defaultIntValue := reflect.ValueOf(defaultValueTag).Convert(field.Type())
					field.Set(defaultIntValue)
				case reflect.String:
					field.SetString(defaultValueTag)
				case reflect.Bool:
					defaultBoolValue := defaultValueTag == "true" || defaultValueTag == "1"
					field.SetBool(defaultBoolValue)
				}
			}
		}
	}
}

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func main() {
	configFile, err := os.Open("/etc/lineq/lineq.cfg")
	if err != nil {
		fmt.Println("Error opening configuration file:", err)
		return
	}
	defer configFile.Close()

	var config Config
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		fmt.Println("Error decoding configuration:", err)
		return
	}

	setDefaults(&config)

	service_mode := config.SERVICE_MODE
	service_vwr_room_table := config.VWR_ROOM_TABLE
	service_tcp_host := config.TCP_HOST
	service_tcp_port := config.TCP_PORT
	service_web_host := config.WEB_HOST
	service_web_port := config.WEB_PORT
	service_target_port := config.TARGET_PORT
	vwr_session_duration := config.VWR_INACTIVITY_DURATION

	initLogger()

	cFlag := flag.Bool("c", false, "generate haproxy configuration (boolean)")
	flag.Parse()

	go initWebServer(service_web_host, service_web_port)
	listen, err := net.Listen("tcp", service_tcp_host+":"+service_tcp_port)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	if service_mode == "vwr" {
		if *cFlag {
			generateHAProxyConfiguration(service_vwr_room_table, service_vwr_users_table, vwr_session_duration, service_web_host, service_web_port, service_tcp_host, service_tcp_port)
			os.Exit(0)
		}

		if len(config.VWR_ROUTES) == 0 {
			fmt.Println("routes is empty")
			return
		}

		initRoomTable(config.VWR_ROUTES, service_vwr_room_table)
		go initCache(vwr_session_duration, config.VWR_ROUTES, service_vwr_room_table)
	}

	defer listen.Close()
	for {
		conn, err := listen.Accept()
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}

		client := &Client{
			conn:   conn,
			reader: bufio.NewReader(conn),
		}
		peers = append(peers, client)
		go client.initConnection(service_mode, vwr_session_duration, service_vwr_room_table, config.VWR_ROUTES)
	}
}

func initRoomTable(routes map[string]Route, roomName string) {
	frequency := [][]int{}
	dType := []int{GPC0}
	tableDefinition := TableDefinition{
		StickTableID: 777,
		Name:         roomName,
		KeyType:      STRING,
		KeyLen:       32,
		DataTypes:    dType,
		Expiry:       24 * 60 * 60 * 1000,
		Frequency:    frequency,
	}

	for name, route := range routes {
		roomTable := Table{
			localUpdateId: 0,
			definition:    tableDefinition,
		}

		var key []byte = []byte(name)

		jsonKey, _ := json.Marshal(&key)
		keyEnc := b64.StdEncoding.EncodeToString(jsonKey)

		roomTable.entries = make(map[string]Entry)
		roomEntry := Entry{
			Key: name,
		}
		roomEntry.Values = make(map[int][]int)
		roomEntry.Values[GPC0] = []int{route.TOTAL_ACTIVE_USERS}
		roomTable.entries[keyEnc] = roomEntry
		tables[roomName] = roomTable
		sortedEntries[name] = make([]string, 0)
	}
}

func initLogger() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.SetPrefix("lineQ   ")
}

func generateHAProxyConfiguration(roomTable string, usersTable string, session_duration int, webHost string, webPort string, tcpHost string, tcpPort string) {
	fileName := "haproxy.cfg"
	config := ""
	file, err := os.Create(fileName)

	if err != nil {
		log.Println("Error creating the file:", err)
		return
	}
	defer file.Close() // Ensure the file is closed when done

	config += fmt.Sprintln("#### (lineQ) HAProxy BASIC Configuration ####")
	config += fmt.Sprintln("#### (lineQ) please adjust this config according to your needs ####")
	config += fmt.Sprintln("peers lineq")
	config += fmt.Sprintln("\tbind 0.0.0.0:55555")
	config += fmt.Sprintln("\tserver haproxy1")
	config += fmt.Sprintf("\tserver lineq %s:%s\n", tcpHost, tcpPort)
	config += fmt.Sprintf("backend %s\n", roomTable)
	config += fmt.Sprintf("\tstick-table type integer size 2 expire 1d store gpc0 peers lineq\n")
	config += fmt.Sprintf("backend %s\n", usersTable)
	config += fmt.Sprintf("\tstick-table type string len 36 size 100k expire %vm store gpc1 peers lineq\n", session_duration)
	config += fmt.Sprintln("frontend fe_main")
	config += fmt.Sprintf("\tbind *:80\n")
	config += fmt.Sprintf("\thttp-request track-sc0 int(1) table %s\n", roomTable)
	config += fmt.Sprintf("\thttp-request set-var(txn.has_cookie) req.cook_cnt(sessionid)\n")
	config += fmt.Sprintf("\thttp-request set-var(txn.t2) uuid()  if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-response add-header Set-Cookie \"sessionid=%%[var(txn.t2)]; path=/\" if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-request set-var(txn.sessionid) req.cook(sessionid)\n")
	config += fmt.Sprintf("\thttp-request track-sc1 var(txn.sessionid) table timestamps if { var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\thttp-request track-sc1 var(txn.t2) table timestamps if !{ var(txn.has_cookie) -m int gt 0 }\n")
	config += fmt.Sprintf("\tacl has_slot sc_get_gpc1(1) eq 1\n")
	config += fmt.Sprintf("\tacl free_slot sc_get_gpc0(0) gt 0\n")
	config += fmt.Sprintf("\thttp-request sc-inc-gpc1(1) if free_slot !has_slot\n")
	config += fmt.Sprintf("\tuse_backend bk_yes if has_slot\n")
	config += fmt.Sprintf("\tdefault_backend bk_no\n")
	config += fmt.Sprintln("backend bk_yes")
	config += fmt.Sprintln("\tmode http")
	config += fmt.Sprintln("\t#### (lineq) change to actual ip:port(s) of your service")
	config += fmt.Sprintln("\tserver server 127.0.0.1:8889")
	config += fmt.Sprintln("backend bk_no")
	config += fmt.Sprintln("\tmode http")
	config += fmt.Sprintf("\tserver lineq %s:%s\n", webHost, webPort)

	data := []byte(config)
	_, err = file.Write(data)
	if err != nil {
		fmt.Println("Error writing to the file:", err)
		return
	}

	fmt.Println("Data has been written to", fileName)
}
