// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package wineventlog

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var insertStrings = newPlaceHolderInserts()

type placeHolderInserts struct {
	InsertStrings [99]*uint16
	EvtVariants   [99]EvtVariant
	ValuesCount   uint32
	ValuesPtr     uintptr
}

func newPlaceHolderInserts() *placeHolderInserts {
	placeholder := &placeHolderInserts{}
	for i := 0; i < len(placeholder.EvtVariants); i++ {
		slice, err := windows.UTF16FromString(`{{ eventParam $ ` + strconv.Itoa(i+1) + ` }}`)
		if err != nil {
			// This will never happen.
			panic(err)
		}

		ptr := &slice[0]
		placeholder.InsertStrings[i] = ptr
		placeholder.EvtVariants[i] = EvtVariant{
			Value: uintptr(unsafe.Pointer(ptr)),
			Count: uint32(len(slice)),
			Type:  EvtVarTypeString,
		}
		placeholder.EvtVariants[i].Type = EvtVarTypeString
	}

	placeholder.ValuesCount = uint32(len(placeholder.EvtVariants))
	placeholder.ValuesPtr = uintptr(unsafe.Pointer(&placeholder.EvtVariants[0]))
	return placeholder
}
