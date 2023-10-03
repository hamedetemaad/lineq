package main

import (
	"bufio"
	b64 "encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
)

type Client struct {
	conn                net.Conn
	reader              *bufio.Reader
	lastTableDefinition TableDefinition
	buffer              []byte
	pointer             int
	mode                string
	tables              map[string]Table
}

func (client *Client) sendHeartBeat() {
	client.conn.Write([]byte{CLASS_CONTROL, HEARTBEAT})
	//time.Sleep(3 * time.Second)
	//client.sendHeartBeat()
}

func (client *Client) sendStatus(remoteId string) {
	client.conn.Write([]byte(SUCCEEDED + "\n"))
}

func (client *Client) initConnection(name string, mode string, auto_sync bool) {
	client.mode = mode
	message, err := client.reader.ReadString('\n')
	if err != nil {
		client.conn.Close()
		return
	}
	fmt.Printf("Message incoming: %s\n", string(message))
	if matches := matchString(`^HAProxyS\s+(\d+(\.\d+)?)`, string(message)); len(matches) > 0 {
		if matches[1] != "2.1" {
			client.conn.Write([]byte(BAD_VERSION + "\n"))
		}
		remoteId, err := client.reader.ReadString('\n')
		if err != nil {
			client.conn.Close()
			return
		}
		if remoteId != name+"\n" {
			client.conn.Write([]byte(REMOTE_ID_MISMATCH + "\n"))
			client.conn.Close()
			return
		}

		peerInfo, _ := client.reader.ReadString('\n')
		if matchesPattern(`.+\s+\d+\s+\d+`, peerInfo) {
			client.sendStatus(remoteId)
			//go client.sendHeartBeat()
			client.tables = make(map[string]Table)
			if auto_sync {
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_REQUEST})
			}
			client.handleRequests()
		}
	} else {
		client.conn.Write([]byte(PROTOCOL_ERROR + "\n"))
	}
	client.conn.Close()
	return
}

func (client *Client) readUpdateAck() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	end := client.pointer + length
	fmt.Println("End ", end)

	client.pointer += consumed

	consumed, stickTableId, _ := decode(client.buffer[client.pointer:])
	fmt.Println("stick table id ", stickTableId, consumed)
	client.pointer += consumed

	updateId := binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4])
	fmt.Println("update id ", updateId)
	client.pointer += 4
}

func (client *Client) readEntryUpdate() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}
	end := client.pointer + length
	fmt.Println("End ", end)

	client.pointer += consumed

	updateId := binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4])
	client.pointer += 4

	tableDefinition := client.lastTableDefinition

	var keyType int
	var keyValue interface{}

	switch tableDefinition.KeyType {
	case IPv4:
		keyType = IPv4
		keyValue = client.buffer[client.pointer : client.pointer+tableDefinition.KeyLen]
		client.pointer += tableDefinition.KeyLen
	case STRING:
		consumed, keyLen, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		keyType = STRING
		keyValue = string(client.buffer[client.pointer : client.pointer+keyLen])
		client.pointer += keyLen
	case SINT:
		keyType = SINT
		keyValue = int32(binary.BigEndian.Uint32(client.buffer[client.pointer : client.pointer+4]))
		client.pointer += 4
	default:
		fmt.Println("error key")
		return
	}

	values := make(map[int][]int)
	for i := 0; i < len(tableDefinition.DataTypes); i++ {
		fmt.Println("DataType[...]", tableDefinition.DataTypes[i])
		switch tableDefinition.DataTypes[i] {
		case SERVER_ID:
			consumed, server_id, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], server_id)
			client.pointer += consumed

			consumed, x, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], x)
			client.pointer += consumed

			consumed, y, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], y)
			client.pointer += consumed

			consumed, nameLen, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], nameLen)
			client.pointer += consumed

			for i := 0; i < nameLen; i++ {
				consumed, char, _ := decode(client.buffer[client.pointer:])
				values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], char)
				client.pointer += consumed
			}
		case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
			consumed, number, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = []int{number}
			client.pointer += consumed
		case HTTP_REQ_RATE:
			consumed, curr_tick, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], curr_tick)
			client.pointer += consumed
			fmt.Println("cur tick", curr_tick)

			consumed, curr_ctr, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], curr_ctr)
			client.pointer += consumed
			fmt.Println("req rate", curr_ctr)

			consumed, prev_ctr, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = append(values[tableDefinition.DataTypes[i]], prev_ctr)
			client.pointer += consumed
			fmt.Println("prev ctr", prev_ctr)
		case BYTES_IN_CNT, BYTES_OUT_CNT:
			consumed, number, _ := decode(client.buffer[client.pointer:])
			values[tableDefinition.DataTypes[i]] = []int{number}
			client.pointer += consumed
		default:
			fmt.Println("error values")
		}
	}


	updateEntry := EntryUpdate{
		UpdateID: updateId,
		Expiry:   0,
		KeyType:  keyType,
		KeyValue: keyValue,
		Values:   values,
	}

	fmt.Printf("update id %v\n", updateEntry.UpdateID)
	fmt.Printf("expiry %v\n", updateEntry.Expiry)
	fmt.Printf("keyType %v\n", updateEntry.KeyType)
	fmt.Printf("keyValue %v\n", updateEntry.KeyValue)
	fmt.Printf("values %v\n", updateEntry.Values)

	fmt.Println("Pointer ", client.pointer)
	fmt.Println("End ", end)
	keyEnc := client.updateTable(updateEntry)

	client.sendUpdateAck(updateId)
	if client.mode == "agg" {
		client.updatePeers(updateEntry.KeyType, updateEntry.KeyValue, keyEnc)
	}
	sendTableUpdate(tableDefinition.Name, keyEnc)
}

func (client *Client) sendUpdateAck(uId uint32) {
	message := make([]byte, 0)

	tableDefinition := client.lastTableDefinition

	header := []byte{CLASS_UPDATE, UPDATE_ACK}
	encodedTableId := encode(tableDefinition.StickTableID)

	updateId := make([]byte, 4)
	binary.BigEndian.PutUint32(updateId, uId)

	data := append(encodedTableId, updateId...)
	length := encode(len(data))
	message = append(header, length...)
	message = append(message, data...)
	client.conn.Write(message)
}

func (client *Client) readTableDefinition() {
	var consumed = 0
	var length = 0

	for {
		consumed, length, _ = decode(client.buffer[client.pointer:])
		if consumed != 0 {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}

	length = length + consumed

	for {
		if len(client.buffer[client.pointer:]) >= length {
			break
		}

		tmp := make([]byte, 512)
		n, err := client.reader.Read(tmp)
		if err != nil {
			client.conn.Close()
			return
		}

		client.buffer = append(client.buffer, tmp[:n]...)
	}
	end := client.pointer + length

	client.pointer += consumed

	consumed, stickTableId, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	fmt.Println("stick consumed ", consumed)

	consumed, nameLength, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	fmt.Printf("name length %v\n", nameLength)

	name := string(client.buffer[client.pointer : client.pointer+nameLength])
	client.pointer += nameLength

	consumed, keyType, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	switch keyType {
	case SINT, IPv4, IPv6, STRING, BINARY:
		fmt.Printf("well\n")
	default:
		fmt.Printf("Incorrect key type %v\n", keyType)
		return
	}

	consumed, keyLen, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	consumed, dataType, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed
	fmt.Println("dataType consumed ", consumed)
	fmt.Println("dataType ", dataType)

	consumed, expiry, _ := decode(client.buffer[client.pointer:])
	client.pointer += consumed

	frequency := [][]int{}
	for {
		i := 0

		if client.pointer >= end {
			break
		}

		var freq []int

		consumed, freq_cnt, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		freq = append(freq, freq_cnt)

		consumed, freq_cnt_prd, _ := decode(client.buffer[client.pointer:])
		client.pointer += consumed
		freq = append(freq, freq_cnt_prd)

		fmt.Printf("counter %v %v\n", freq[0], freq[1])

		frequency = append(frequency, freq)

		i++
	}

	types := [19]int{SERVER_ID, GPT0, GPC0, GPC0_RATE, CONN_CNT, CONN_RATE, CONN_CUR, SESS_CNT, SESS_RATE, HTTP_REQ_CNT,
		HTTP_REQ_RATE, HTTP_ERR_CNT, HTTP_ERR_RATE, BYTES_IN_CNT, BYTES_IN_RATE, BYTES_OUT_CNT, BYTES_OUT_RATE, GPC1, GPC1_RATE}

	dTypes := []int{}

	for i := 0; i < len(types); i++ {
		if ((dataType >> types[i]) & 1) != 0 {
			dTypes = append(dTypes, types[i])
		}
	}

	tableDefinition := TableDefinition{
		StickTableID: stickTableId,
		Name:         name,
		KeyType:      keyType,
		KeyLen:       keyLen,
		DataTypes: dTypes,
		Expiry:    expiry,
		Frequency: frequency,
	}

	fmt.Println("StickTableId ", tableDefinition.StickTableID)
	fmt.Println("Name ", tableDefinition.Name)
	fmt.Println("KeyType ", tableDefinition.KeyType)
	fmt.Println("KeyLen ", tableDefinition.KeyLen)
	fmt.Println("DataTypes ", tableDefinition.DataTypes)
	fmt.Println("Expiry ", tableDefinition.Expiry)
	fmt.Println("Frequency ", tableDefinition.Frequency)

	client.lastTableDefinition = tableDefinition

	if _, exists := client.tables[name]; !exists {
		table := Table{
			localUpdateId: 0,
			definition:    tableDefinition,
		}
		client.tables[name] = table
	}

	return
}

func (client *Client) updateTable(entryUpdate EntryUpdate) string {
	tableDefinition := client.lastTableDefinition

	name := tableDefinition.Name
	entry := Entry{
		Key:    entryUpdate.KeyValue,
		Values: entryUpdate.Values,
	}

	table := client.tables[name]

	var key []byte
	switch v := entry.Key.(type) {
	case string:
		key = []byte(v)
	case int32:
		key = s32tob(v)
	case []byte:
		key = v
	default:
		fmt.Println("Unsupported type")
	}

	jsonKey, _ := json.Marshal(&key)
	keyEnc := b64.StdEncoding.EncodeToString(jsonKey)

	if table.entries == nil {
		table.entries = make(map[string]Entry)
	}
	table.entries[keyEnc] = entry

	client.tables[name] = table

	if _, exists := tables[name]; !exists {
		tables[name] = Table{
			localUpdateId: 0,
			definition:    tableDefinition,
		}
	}

	if client.mode == "agg" {
		tables[name] = table
	} else if client.mode == "acc" {
		globTable := tables[name]
		if globTable.entries == nil {
			globTable.entries = make(map[string]Entry)
		}

		globEntry := Entry{
			Key: entryUpdate.KeyValue,
		}

		for i := 0; i < len(tableDefinition.DataTypes); i++ {
			dataType := tableDefinition.DataTypes[i]
			switch dataType {
			case SERVER_ID:
				globEntry.Values[dataType] = make([]int, 0)
			case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
				globEntry.Values[dataType] = make([]int, 1)
			case HTTP_REQ_RATE:
				globEntry.Values[dataType] = make([]int, 3)
			case BYTES_IN_CNT, BYTES_OUT_CNT:
				globEntry.Values[dataType] = make([]int, 1)
			default:
				fmt.Println("error values")
			}
		}

		for i := 0; i < len(tableDefinition.DataTypes); i++ {
			for i := 0; i < len(peers); i++ {
				if locTable, exists := peers[i].tables[name]; exists {
					if locEnt, exists := locTable.entries[keyEnc]; exists {
						dType := tableDefinition.DataTypes[i]
						switch dType {
						case SERVER_ID:
						case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
						case HTTP_REQ_RATE:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
							globEntry.Values[dType][1] += locEnt.Values[dType][1]
							globEntry.Values[dType][2] += locEnt.Values[dType][2]
						case BYTES_IN_CNT, BYTES_OUT_CNT:
							globEntry.Values[dType][0] += locEnt.Values[dType][0]
						default:
							fmt.Println("error values")
						}
					}
				}
			}
		}

		globTable.entries[keyEnc] = globEntry
		tables[name] = globTable
	}
	return keyEnc
}

func (client *Client) createTableDefinition(tableDefinition TableDefinition) []byte {
	message := make([]byte, 0)
	stickTableId := encode(tableDefinition.StickTableID)
	tableNameLen := encode(len(tableDefinition.Name))
	message = append(stickTableId, tableNameLen...)
	name := []byte(tableDefinition.Name)
	message = append(message, name...)
	keyType := encode(tableDefinition.KeyType)
	message = append(message, keyType...)
	keyLen := encode(tableDefinition.KeyLen)
	message = append(message, keyLen...)

	dataType := 0

	for i := 0; i < len(tableDefinition.DataTypes); i++ {
		dataType = dataType | (1 << tableDefinition.DataTypes[i])
	}

	dataTypeBitFieald := encode(dataType)
	message = append(message, dataTypeBitFieald...)
	expiry := encode(tableDefinition.Expiry)
	message = append(message, expiry...)

	for i := 0; i < len(tableDefinition.Frequency); i++ {
		freq_cnt := encode(tableDefinition.Frequency[i][0])
		message = append(message, freq_cnt...)
		freq_cnt_prd := encode(tableDefinition.Frequency[i][1])
		message = append(message, freq_cnt_prd...)
	}
	return message
}

func (client *Client) createEntryUpdate(tableDef TableDefinition, keyType int, keyValue interface{}, keyEnc string) []byte {
	message := make([]byte, 0)
	tableName := tableDef.Name
	table := tables[tableName]
	table.localUpdateId += 1
	entry := table.entries[keyEnc]

	localUpdateId := make([]byte, 4)
	binary.BigEndian.PutUint32(localUpdateId, table.localUpdateId)
	message = append(message, localUpdateId...)

	switch keyType {
	case SINT:
		if value, ok := keyValue.(int32); ok {
			message = append(message, s32tob(value)...)
		} else {
			fmt.Println("Conversion to int32 failed.")
			return nil
		}
	case IPv4:
		fmt.Println("IPv4")
		if value, ok := keyValue.([]byte); ok {
			message = append(message, value...)

		} else {
			fmt.Println("Conversion to []byte failed.")
			return nil
		}
	case IPv6:
	case STRING:
		if value, ok := keyValue.(string); ok {
			message = append(message, encode(len(value))...)
			message = append(message, value...)
		}
	case BINARY:
	default:
		fmt.Printf("Incorrect key type %v\n", keyType)
		return nil
	}

	for i := 0; i < len(tableDef.DataTypes); i++ {
		dataType := tableDef.DataTypes[i]
		switch dataType {
		case SERVER_ID:
			message = append(message, encode(entry.Values[dataType][0])...)
			message = append(message, encode(entry.Values[dataType][1])...)
			message = append(message, encode(entry.Values[dataType][2])...)
			message = append(message, encode(entry.Values[dataType][3])...)

			for i := 0; i < entry.Values[dataType][3]; i++ {
				message = append(message, encode(entry.Values[dataType][4+i])...)
			}
		case GPT0, GPC0, CONN_CNT, CONN_CUR, SESS_CNT, HTTP_REQ_CNT, HTTP_ERR_CNT, GPC1:
			message = append(message, encode(entry.Values[dataType][0])...)
		case HTTP_REQ_RATE:
			cur_tick := encode(entry.Values[dataType][0])
			fmt.Println(entry.Values[dataType][0])
			message = append(message, cur_tick...)

			cur_ctr := encode(entry.Values[dataType][1])
			fmt.Println(entry.Values[dataType][1])
			message = append(message, cur_ctr...)

			prev_ctr := encode(entry.Values[dataType][2])
			fmt.Println(entry.Values[dataType][2])
			message = append(message, prev_ctr...)
		case BYTES_IN_CNT, BYTES_OUT_CNT:
		default:
			fmt.Println("unknown type")
		}
	}
	return message
}

func (client *Client) updatePeer() {
	for key, value := range tables {
		keyType := tables[key].definition.KeyType
		tableDef := client.createTableDefinition(tables[key].definition)
		for keyEnc, entry := range value.entries {
			keyValue := entry.Key
			entryDef := client.createEntryUpdate(tables[key].definition, keyType, keyValue, keyEnc)
			client.sendUpdate(tableDef, entryDef, true)
		}
	}
}

func (client *Client) sendUpdate(tableDef []byte, entryDef []byte, local bool) {
	message := make([]byte, 0)

	tableHeader := []byte{CLASS_UPDATE, STICK_TABLE_DEFINITION}
	tableLength := encode(len(tableDef))
	message = append(tableHeader, tableLength...)
	message = append(message, tableDef...)

	entryHeader := []byte{CLASS_UPDATE, ENTRY_UPDATE}
	entryLength := encode(len(entryDef))
	message = append(message, entryHeader...)
	message = append(message, entryLength...)
	message = append(message, entryDef...)

	if local {
		client.conn.Write(message)
	} else {
		for i := 0; i < len(peers); i++ {
			peers[i].conn.Write(message)
		}
	}
}

func (client *Client) updatePeers(keyType int, keyValue interface{}, keyEnc string) {
	tableDef := client.createTableDefinition(client.lastTableDefinition)
	entryDef := client.createEntryUpdate(client.lastTableDefinition, keyType, keyValue, keyEnc)

	client.sendUpdate(tableDef, entryDef, false)
}

func (client *Client) handleRequests() {
	client.buffer = make([]byte, 0)
	client.pointer = 0
	for {
		if client.pointer >= len(client.buffer) {
			tmp := make([]byte, 128)
			n, err := client.reader.Read(tmp)
			if err != nil {
				client.conn.Close()
				return
			}
			client.buffer = tmp[:n]
			client.pointer = 0
		} else if len(client.buffer) == 1 {
			tmp := make([]byte, 128)
			n, err := client.reader.Read(tmp)
			if err != nil {
				client.conn.Close()
				return
			}
			client.buffer = append(client.buffer, tmp[:n]...)
		}

		reqClass := client.buffer[client.pointer]
		client.pointer += 1

		switch reqClass {
		case CLASS_CONTROL:
			fmt.Println("control class")
			classType := client.buffer[client.pointer]
			client.pointer += 1
			switch classType {
			case HEARTBEAT:
				fmt.Println("heartbeat")
				client.sendHeartBeat()
			case SYNCHRONIZATION_REQUEST:
				fmt.Println("synchronization request")
				client.updatePeer()
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_FINISHED})
			case SYNCHRONIZATION_PARTIAL:
				fmt.Println("synchronization partial")
				//client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_FINISHED})
				client.conn.Write([]byte{CLASS_CONTROL, SYNCHRONIZATION_CONFIRMED})
			case SYNCHRONIZATION_CONFIRMED:
				fmt.Println("synchronization confirmed")
			}
		case CLASS_ERROR:
			fmt.Println("error class")
			classType := client.buffer[client.pointer]
			client.pointer += 1
			if classType == 0 {
				fmt.Println("protocol error")
			} else {
				fmt.Println("size limit error")
			}
		case CLASS_UPDATE:
			fmt.Println("stick-table updates class")
			classType := client.buffer[client.pointer]
			client.pointer += 1

			switch classType {
			case ENTRY_UPDATE:
				fmt.Println("entry update")
				client.readEntryUpdate()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			case INCREMENTAL_ENTRY_UPDATE:
				fmt.Println("incremental entry update")
			case STICK_TABLE_DEFINITION:
				fmt.Println("stick table definition")
				client.readTableDefinition()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			case STICK_TABLE_SWITCH:
				fmt.Println("stick table switch")
			case UPDATE_ACK:
				fmt.Println("update message acknowledgement")
				client.readUpdateAck()
				if client.pointer < len(client.buffer) {
					client.buffer = client.buffer[client.pointer:]
					client.pointer = 0
				}
			}
		case CLASS_RESERVED:
			fmt.Println("reserved class")
		default:
			fmt.Println("class does not implemented ", reqClass)
		}
	}
}
