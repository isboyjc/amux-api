package dto

import (
	"math"
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

type IntValue int

func (i *IntValue) UnmarshalJSON(b []byte) error {
	var n int
	if err := common.Unmarshal(b, &n); err == nil {
		*i = IntValue(n)
		return nil
	}
	var s string
	if err := common.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*i = IntValue(v)
	return nil
}

func (i IntValue) MarshalJSON() ([]byte, error) {
	return common.Marshal(int(i))
}

// FloatValue 兼容上游将浮点数编码为 JSON number 或数字字符串的响应。
type FloatValue float64

func (f *FloatValue) UnmarshalJSON(data []byte) error {
	var number float64
	if err := common.Unmarshal(data, &number); err == nil {
		*f = FloatValue(number)
		return nil
	}

	var str string
	if err := common.Unmarshal(data, &str); err != nil {
		return err
	}
	value, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return err
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return strconv.ErrSyntax
	}
	*f = FloatValue(value)
	return nil
}

func (f FloatValue) MarshalJSON() ([]byte, error) {
	return common.Marshal(float64(f))
}

type BoolValue bool

func (b *BoolValue) UnmarshalJSON(data []byte) error {
	var boolean bool
	if err := common.Unmarshal(data, &boolean); err == nil {
		*b = BoolValue(boolean)
		return nil
	}
	var str string
	if err := common.Unmarshal(data, &str); err != nil {
		return err
	}
	if str == "true" {
		*b = BoolValue(true)
	} else if str == "false" {
		*b = BoolValue(false)
	} else {
		return common.Unmarshal(data, &boolean)
	}
	return nil
}
func (b BoolValue) MarshalJSON() ([]byte, error) {
	return common.Marshal(bool(b))
}
