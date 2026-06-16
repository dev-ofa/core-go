package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/dev-ofa/core-go/model/datax"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/x/bsonx/bsoncore"
)

// SnowflakeID stores a snowflake-style ID as a decimal string in Go/API paths,
// while serializing to databases as a numeric value so database ordering follows numeric ordering.
//
// This type is useful when callers need API-safe strings but still want numeric
// database storage. It has a higher integration cost than plain fixed-width
// string IDs because every persistence layer must honor the custom codec. When a
// project can choose its ID format freely, prefer fixed-width lexicographic IDs
// so application code does not need numeric/string conversion at every boundary.
type SnowflakeID string

// NewSnowflakeID converts a uint64 ID into SnowflakeID.
func NewSnowflakeID(id uint64) SnowflakeID {
	return SnowflakeID(strconv.FormatUint(id, 10))
}

// ParseSnowflakeID parses a decimal string into SnowflakeID.
func ParseSnowflakeID(s string) (SnowflakeID, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return "", datax.NewValidationError("snowflake id should not be empty", nil, nil)
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return "", datax.NewValidationError("parse snowflake id failed", nil, err)
	}
	return NewSnowflakeID(id), nil
}

// Uint64 returns the numeric ID value.
func (id SnowflakeID) Uint64() (uint64, error) {
	if id == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(string(id), 10, 64)
	if err != nil {
		return 0, datax.NewValidationError("parse snowflake id failed", nil, err)
	}
	return v, nil
}

// Int64 returns the numeric ID value as int64 when it fits common database integer storage.
func (id SnowflakeID) Int64() (int64, error) {
	v, err := id.Uint64()
	if err != nil {
		return 0, err
	}
	if v > math.MaxInt64 {
		return 0, datax.NewValidationError(fmt.Sprintf("snowflake id %d exceeds int64 range", v), nil, nil)
	}
	return int64(v), nil
}

// String returns the decimal string form.
func (id SnowflakeID) String() string {
	return string(id)
}

// MarshalJSON serializes the ID as a JSON string.
func (id SnowflakeID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON accepts a JSON string, and also accepts a JSON number for controlled server-side use.
func (id *SnowflakeID) UnmarshalJSON(data []byte) error {
	if id == nil {
		return datax.NewValidationError("unmarshal snowflake id into nil pointer", nil, nil)
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		parsed, parseErr := ParseSnowflakeID(s)
		if parseErr != nil {
			return parseErr
		}
		*id = parsed
		return nil
	}
	var n uint64
	if err := json.Unmarshal(data, &n); err != nil {
		return datax.NewValidationError("unmarshal snowflake id failed", nil, err)
	}
	*id = NewSnowflakeID(n)
	return nil
}

// MarshalBSONValue stores the ID as BSON int64 so database ordering follows numeric ordering.
func (id SnowflakeID) MarshalBSONValue() (byte, []byte, error) {
	v, err := id.Int64()
	if err != nil {
		return 0, nil, err
	}
	return byte(bson.TypeInt64), bsoncore.AppendInt64(nil, v), nil
}

// UnmarshalBSONValue decodes BSON int64/int32/string values into SnowflakeID.
func (id *SnowflakeID) UnmarshalBSONValue(typ byte, data []byte) error {
	if id == nil {
		return datax.NewValidationError("unmarshal snowflake id into nil pointer", nil, nil)
	}
	switch bson.Type(typ) {
	case bson.TypeInt64:
		v, _, ok := bsoncore.ReadInt64(data)
		if !ok {
			return datax.NewValidationError("read int64 snowflake id failed", nil, nil)
		}
		if v < 0 {
			return datax.NewValidationError(fmt.Sprintf("snowflake id must not be negative: %d", v), nil, nil)
		}
		*id = NewSnowflakeID(uint64(v))
		return nil
	case bson.TypeInt32:
		v, _, ok := bsoncore.ReadInt32(data)
		if !ok {
			return datax.NewValidationError("read int32 snowflake id failed", nil, nil)
		}
		if v < 0 {
			return datax.NewValidationError(fmt.Sprintf("snowflake id must not be negative: %d", v), nil, nil)
		}
		*id = NewSnowflakeID(uint64(v))
		return nil
	case bson.TypeString:
		v, _, ok := bsoncore.ReadString(data)
		if !ok {
			return datax.NewValidationError("read string snowflake id failed", nil, nil)
		}
		parsed, err := ParseSnowflakeID(v)
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	default:
		return datax.NewValidationError(fmt.Sprintf("unsupported snowflake id bson type %s", bson.Type(typ).String()), nil, nil)
	}
}

// Value stores SnowflakeID as int64 for SQL drivers.
func (id SnowflakeID) Value() (driver.Value, error) {
	return id.Int64()
}

// Scan decodes SnowflakeID from SQL driver values.
func (id *SnowflakeID) Scan(src any) error {
	if id == nil {
		return datax.NewValidationError("scan snowflake id into nil pointer", nil, nil)
	}
	switch v := src.(type) {
	case int64:
		if v < 0 {
			return datax.NewValidationError(fmt.Sprintf("snowflake id must not be negative: %d", v), nil, nil)
		}
		*id = NewSnowflakeID(uint64(v))
		return nil
	case int:
		if v < 0 {
			return datax.NewValidationError(fmt.Sprintf("snowflake id must not be negative: %d", v), nil, nil)
		}
		*id = NewSnowflakeID(uint64(v))
		return nil
	case uint64:
		*id = NewSnowflakeID(v)
		return nil
	case string:
		parsed, err := ParseSnowflakeID(v)
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	case []byte:
		parsed, err := ParseSnowflakeID(string(v))
		if err != nil {
			return err
		}
		*id = parsed
		return nil
	case nil:
		*id = ""
		return nil
	default:
		return datax.NewValidationError(fmt.Sprintf("unsupported snowflake id scan type %T", src), nil, nil)
	}
}
