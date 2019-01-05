// Copyright 2019 The Morning Consult, LLC or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//         https://www.apache.org/licenses/LICENSE-2.0
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/docker/docker-credential-helpers/credentials"
	"github.com/morningconsult/docker-credential-vault-login/helper"
	"github.com/morningconsult/docker-credential-vault-login/version"
)

const banner = "Docker Credential Helper for Vault Storage version %v, commit %v, built %v\n"

func main() {
	var versionFlag bool
	flag.BoolVar(&versionFlag, "version", false, "print version and exit")
	flag.Parse()

	// Exit safely when version is used
	if versionFlag {
		fmt.Printf(banner, version.Version, version.Commit, version.Date)
		os.Exit(0)
	}
	credentials.Serve(helper.NewHelper(nil))
}
