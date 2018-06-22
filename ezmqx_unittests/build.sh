###############################################################################
# Copyright 2018 Samsung Electronics All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
###############################################################################

#!/bin/bash
cd ./../../../
PROJECT_ROOT=$(pwd)
export GOPATH=$(pwd)
export CGO_CFLAGS=-I$PWD/dependencies/datamodel-aml-go/dependencies/datamodel-aml-c/include/
export CGO_LDFLAGS=-L$PWD/src/go/ezmqx_extlibs
export CGO_LDFLAGS=$CGO_LDFLAGS" -lcaml -laml"

cd "$PROJECT_ROOT/src/go/ezmqx_unittests"

export LD_LIBRARY_PATH=../ezmqx_extlibs

# Run the unit testcases
echo -e "=======  Run unit testcases"
go test -v

# Run the unit testcases and generate a coverage report
echo -e "======= Generate code coverage report"
go test -coverpkg go/ezmqx -coverprofile=coverage.out

