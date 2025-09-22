/**
 * Copyright 2023 The KusionStack Authors
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

package initializers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"kusionstack.io/rollout/pkg/controllers/rollout"
	"kusionstack.io/rollout/pkg/controllers/rolloutrun"
	"kusionstack.io/rollout/pkg/controllers/scalerun"
)

func init() {
	// init rollout controller
	utilruntime.Must(Controllers.Add(rollout.ControllerName, rollout.InitFunc))

	// init rolloutRun controller
	utilruntime.Must(Controllers.Add(rolloutrun.ControllerName, rolloutrun.InitFunc))

	// init scaleRun controller
	utilruntime.Must(Controllers.Add(scalerun.ControllerName, scalerun.InitFunc))
}
