/*
 * Copyright 2023 The KusionStack Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1alpha1

import (
	"encoding/json"
	"fmt"
)

// Params is a list of parameters
type Params []Param

// Param defines the parameter of a task
type Param struct {
	// Name is the name of the parameter
	Name string `json:"name"`
	// Value is the value of the parameter
	Value ParamValue `json:"value"`
}

type ParamValueType string

const (
	ParamValueTypeString ParamValueType = "string"
	ParamValueTypeInt    ParamValueType = "int"
	ParamValueTypeBool   ParamValueType = "bool"
	ParamValueTypeArray  ParamValueType = "array"
	ParamValueTypeObject ParamValueType = "object"
)

// ParamValue is the value of a parameter
// +kubebuilder:validation:Type=string
// +kubebuilder:pruning:PreserveUnknownFields
type ParamValue struct {
	Type ParamValueType `json:"type"`
	// StringValue is the string value of the parameter
	StringValue string `json:"stringValue"`
	// IntValue is the int value of the parameter
	IntValue int `json:"intValue"`
	// BoolValue is the bool value of the parameter
	BoolValue bool `json:"boolValue"`
	// ArrayValue is the array value of the parameter
	// +listType=atomic
	ArrayValue []string `json:"arrayValue"`
	// ObjectValue is the object value of the parameter
	ObjectValue map[string]string `json:"objectValue"`
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (pv *ParamValue) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		pv.Type = ParamValueTypeString
		return nil
	}

	if data[0] == '[' {
		var a []string
		if err := json.Unmarshal(data, &a); err == nil {
			pv.Type = ParamValueTypeArray
			pv.ArrayValue = a
			return nil
		}
	}

	if data[0] == '{' {
		var o map[string]string
		if err := json.Unmarshal(data, &o); err == nil {
			pv.Type = ParamValueTypeObject
			pv.ObjectValue = o
			return nil
		}
	}

	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		pv.Type = ParamValueTypeInt
		pv.IntValue = i
		return nil
	}

	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		pv.Type = ParamValueTypeBool
		pv.BoolValue = b
		return nil
	}

	// by default, we assume it's a string
	pv.Type = ParamValueTypeString
	if err := json.Unmarshal(data, &pv.StringValue); err == nil {
		return nil
	}
	pv.StringValue = string(data)

	return nil
}

// MarshalJSON implements the json.Marshaler interface.
func (pv ParamValue) MarshalJSON() ([]byte, error) {
	switch pv.Type {
	case ParamValueTypeString:
		return json.Marshal(pv.StringValue)
	case ParamValueTypeInt:
		return json.Marshal(pv.IntValue)
	case ParamValueTypeBool:
		return json.Marshal(pv.BoolValue)
	case ParamValueTypeArray:
		return json.Marshal(pv.ArrayValue)
	case ParamValueTypeObject:
		return json.Marshal(pv.ObjectValue)
	default:
		return nil, fmt.Errorf("unknown param value type: %s", pv.Type)
	}
}
