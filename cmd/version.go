/*
 *
 * Copyright 2020-present Arpabet Inc.
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
 *
 */

package cmd

import (
	"fmt"
	"github.com/consensusdb/consensusdb/pkg/constants"
)


type versionCommand struct {
}

func (t *versionCommand) Desc() string {
	return "show version"
}

func (t *versionCommand) Run(args []string) error {

	appInfo := constants.GetAppInfo()
	fmt.Printf("ConsensusDB [Version %s, Build %s]\n", appInfo.Version, appInfo.Build)
	fmt.Printf("%s\n", constants.Copyright)
	return nil
}
