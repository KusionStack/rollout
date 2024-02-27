# Quick Start

For demo purpose, this tutorial introduces how to setup KusionStack Rollout on local Kubernetes by kind.


## Prerequisites

Please install the following tools:
- [go](https://go.dev/doc/install)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

## Quick Start

Build the environment locally by running the script

```
bash hack/local-demo.sh
```

The script performs the following tasks:
1. Create a local kubernetes cluster named `kusionstack-rollout` by `kind`
2. Install all rollout CustomResourceDefinations
3. Compile rollout controller, build image and load it into kind cluster
4. Run rollout controller on local kubernetes cluster
5. Create a rollout CR named `rollout-demo` and two statefulsets named `rollout-demo1` and `rollout-demo2`


After script finished:
- you can check statefulsets in default namespace by 
```shell
kubectl --context kind-kusionstack-rollout -n default get statefulsets                                
```

- you can check rollout in default namespace by 
```bash
kubectl --context kind-kusionstack-rollout -n default get rollouts -oyaml
```
The status should be

```yaml
status:
  conditions:
  - lastTransitionTime: "2024-01-09T12:08:02Z"
    lastUpdateTime: "2024-01-09T12:08:02Z"
    message: rollout is not triggered
    reason: UnTriggered
    status: "False"
    type: Progressing
  lastUpdateTime: "2024-01-09T12:08:02Z"
  observedGeneration: 1
  phase: Initialized
```


Then update all statefulsets by:
```bash
kubectl --context kind-kusionstack-rollout apply -f hack/testdata/local-up/update-workloads.yaml
```

Check the rollout status
```bash
kubectl --context kind-kusionstack-rollout -n default get rollouts -oyaml
```

If you see the following status, that means the rollout is paused now.
```yaml
status:
  batchStatus:
    currentBatchIndex: 1
    currentBatchState: Paused
```

Run following command to let rollout continue:
```bash
kubectl --context kind-kusionstack-rollout annotate rollout rollout-demo --overwrite rollout.kusionstack.io/manual-command="resume"
```

Check status again, finally you can see:
```yaml
status:
  batchStatus:
    currentBatchIndex: 2
    currentBatchState: Succeeded
conditions:
- message: rolloutRun is completed
  reason: Completed
  status: "False"
  type: Progressing
```

Finally, you can clean up the whole test enviroment:
```bash
kind delete cluster --name kusionstack-rollout
```
