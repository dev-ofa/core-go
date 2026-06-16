package model

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestSnowflakeIDJSON(t *testing.T) {
	id := NewSnowflakeID(623949464310157351)

	data, err := json.Marshal(struct {
		ID SnowflakeID `json:"id"`
	}{ID: id})
	require.NoError(t, err)
	require.JSONEq(t, `{"id":"623949464310157351"}`, string(data))

	var decoded struct {
		ID SnowflakeID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, id, decoded.ID)
	require.NoError(t, json.Unmarshal([]byte(`{"id":623949464310157351}`), &decoded))
	require.Equal(t, id, decoded.ID)
}

func TestSnowflakeIDBSON(t *testing.T) {
	type doc struct {
		ID SnowflakeID `bson:"_id"`
	}

	id := NewSnowflakeID(623949464310157351)
	data, err := bson.Marshal(doc{ID: id})
	require.NoError(t, err)

	var raw bson.Raw
	require.NoError(t, bson.Unmarshal(data, &raw))
	rawID := raw.Lookup("_id")
	require.Equal(t, bson.TypeInt64, rawID.Type)
	require.Equal(t, int64(623949464310157351), rawID.Int64())

	var decoded doc
	require.NoError(t, bson.Unmarshal(data, &decoded))
	require.Equal(t, id, decoded.ID)
}

func TestSnowflakeIDSQLValueAndScan(t *testing.T) {
	id := NewSnowflakeID(623949464310157351)

	value, err := id.Value()
	require.NoError(t, err)
	require.Equal(t, driver.Value(int64(623949464310157351)), value)

	var scanned SnowflakeID
	require.NoError(t, scanned.Scan(int64(623949464310157351)))
	require.Equal(t, id, scanned)
	require.NoError(t, scanned.Scan(id.String()))
	require.Equal(t, id, scanned)
	require.NoError(t, scanned.Scan([]byte(id.String())))
	require.Equal(t, id, scanned)
}

func TestSnowflakeIDEntityType(t *testing.T) {
	type snowflakeEntity struct {
		Entity[SnowflakeID] `bson:"inline"`
	}

	var _ EntityConstraint[SnowflakeID] = (*snowflakeEntity)(nil)
	entity := &snowflakeEntity{}
	entity.SetID(NewSnowflakeID(42))
	require.Equal(t, NewSnowflakeID(42), entity.GetID())
}
