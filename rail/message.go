package rail

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	//	"github.com/ngaut/log"
	"github.com/siddontang/go-mysql/canal"
)

const (
	MsgIDLength       = 16
	minValidMsgLength = MsgIDLength + 8 + 2 // Timestamp + Attempts
)

type MessageID [MsgIDLength]byte

//Message 消息基类
type Message struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"timestamp"`
	Attempts  uint16 `json:"attempts"`

	// for in-flight handling
	deliveryTS time.Time
	pri        int64
	index      int
	deferred   time.Duration

	Action      string                   `json:"action"`
	Schema      string                   `json:"schema"`
	Table       string                   `json:"table"`
	Rows        []map[string]interface{} `json:"rows"`     //保存目前的数据
	RawRows     []map[string]interface{} `json:"raw_rows"` //保存更新前的数据，只有update操作才有
	PrimaryKeys [][]interface{}          `json:"primary_keys"`
}

//NewMessage 初始化消息
func NewMessage(id string, re *canal.RowsEvent) *Message {
	m := new(Message)
	m.ID = id
	m.Timestamp = int64(time.Now().UnixNano())
	m.Attempts = 0

	m.Rows = make([]map[string]interface{}, 0)
	m.Action = re.Action
	m.Schema = re.Table.Schema
	m.Table = re.Table.Name

	m.PrimaryKeys = make([][]interface{}, len(re.Rows))
	for index, row := range re.Rows {
		pk := make([]interface{}, 0, len(re.Table.PKColumns))

		for _, pkIndex := range re.Table.PKColumns {
			pk = append(pk, row[pkIndex])
		}
		m.PrimaryKeys[index] = pk
	}

	fields := make([]string, 0)
	for _, column := range re.Table.Columns {
		fields = append(fields, column.Name)
	}

	if m.Action == canal.UpdateAction {
		m.RawRows = make([]map[string]interface{}, 0)
		for index, row := range re.Rows {
			if index%2 == 0 {
				m.RawRows = append(m.RawRows, parseRow(row, fields))
			} else {
				m.Rows = append(m.Rows, parseRow(row, fields))
			}
		}
	} else {
		for _, row := range re.Rows {
			m.Rows = append(m.Rows, parseRow(row, fields))
		}
	}

	return m
}

func parseRow(values []interface{}, fields []string) map[string]interface{} {
	rowMap := make(map[string]interface{}, len(values))
	for i, value := range values {
		rowMap[fields[i]] = value
	}
	return rowMap
}

//Encode2Json 输出json格式的消息
func (m *Message) Encode2Json() ([]byte, error) {
	return json.Marshal(m)
}

//Encode2IOReader reader
func (m *Message) Encode2IOReader() (io.Reader, error) {
	b, err := json.Marshal(m)

	if err != nil {
		return nil, err
	}

	return bytes.NewReader(b), nil
}

func writeMessageToBackend(buf *bytes.Buffer, msg *Message, bq BackendQueue) error {
	buf.Reset()
	_, err := msg.WriteTo(buf)
	if err != nil {
		return err
	}
	return bq.Put(buf.Bytes())
}

func (m *Message) WriteTo(w io.Writer) (int, error) {
	jsonBytes, _ := m.Encode2Json()
	return w.Write(jsonBytes)
}

func (m *Message) Brief() string {
	b, _ := json.Marshal(m.PrimaryKeys)
	return string(b)
}

func (m *Message) Detail() string {
	b, _ := json.Marshal(m)
	return string(b)
}

func decodeJson2Message(data []byte) (*Message, error) {
	m := &Message{}
	err := json.Unmarshal(data, m)
	return m, err
}
