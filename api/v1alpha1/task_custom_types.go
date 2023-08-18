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

// CustomTask is defined by the user which will be executed by user's executor server
type CustomTask struct {
	// Name is the name of the custom task
	Name string `json:"name"`

	// Executor is the executor that will execute the custom task
	Executor Executor `json:"executor"`
}

// Executor is the executor that will execute the custom task
type Executor struct {
	// Endpoint is the endpoint of the executor server
	Endpoint string `json:"endpoint"`
}
