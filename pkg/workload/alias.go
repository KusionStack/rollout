package workload

const (
	UpdatedUnreadyGenerationMismatched          UpdatedUnreadyReason = "workload Generation and ObservedGeneration are mismatched"
	UpdatedUnreadyAvailableReplicasNotSatisfied UpdatedUnreadyReason = "updated available replicas is not satisfied"
	UpdatedUnreadyObservedReplicasNotSatisfied  UpdatedUnreadyReason = "observed replicas is not satisfied"
)

type UpdatedUnreadyReason string
