package controller

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	platformv1alpha1 "github.com/tabed23/doks-capacity-operator/api/v1alpha1"
	"github.com/tabed23/doks-capacity-operator/internal/do"
)

// reconcileEvery is the steady-state polling interval (the 60s spec).
const reconcileEvery = 60 * time.Second

// DOClient is the slice of the DigitalOcean API this controller needs. Defined
// as an interface so it can be faked in tests.
type DOClient interface {
	ResolvePool(ctx context.Context, clusterID, poolID, poolName string) (*do.Pool, error)
	SetMaxNodes(ctx context.Context, clusterID, poolID, name string, minNodes, maxNodes int) error
}

// DoksCapacityOperatorReconciler reconciles an OsirsCapacityOperator object.
type DoksCapacityOperatorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	DO     DOClient
}

// +kubebuilder:rbac:groups=platform.mahy.love,resources=dokscapacityoperators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=platform.mahy.love,resources=dokscapacityoperators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=platform.mahy.love,resources=dokscapacityoperators/finalizers,verbs=update

func (r *DoksCapacityOperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var policy platformv1alpha1.DoksCapacityOperator
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Steady-state requeue. Individual branches may shorten this.
	requeue := ctrl.Result{RequeueAfter: reconcileEvery}

	// 1. Resolve the pool (auto-discovering the cluster ID if needed).
	pool, err := r.DO.ResolvePool(ctx, policy.Spec.ClusterID, policy.Spec.PoolID, policy.Spec.PoolName)
	if err != nil {
		l.Error(err, "resolving node pool")
		return requeue, r.fail(ctx, &policy, "ResolveFailed", err.Error())
	}

	policy.Status.ResolvedClusterID = pool.ClusterID
	policy.Status.ResolvedPoolID = pool.PoolID
	policy.Status.CurrentNodeCount = pool.Count
	policy.Status.CurrentMaxNodes = pool.MaxNodes

	// 2. Raising the max is meaningless unless DO autoscaling is on.
	if !pool.AutoScale {
		msg := "pool autoscaling is disabled; enable DigitalOcean autoscaling so a raised max actually adds nodes"
		l.Info(msg, "pool", pool.PoolID)
		return requeue, r.update(ctx, &policy, "Blocked", "AutoscaleDisabled", msg, metav1.ConditionFalse)
	}

	headroom := pool.MaxNodes - pool.Count
	l.Info("observed",
		"count", pool.Count, "poolMax", pool.MaxNodes,
		"headroom", headroom, "ceiling", policy.Spec.MaxNodes)

	// 3. Enough headroom? Nothing to do.
	if headroom > policy.Spec.TriggerFreeNodes {
		msg := fmt.Sprintf("headroom %d > trigger %d", headroom, policy.Spec.TriggerFreeNodes)
		return requeue, r.update(ctx, &policy, "Stable", "Healthy", msg, metav1.ConditionTrue)
	}

	// 4. Trigger hit — but respect cooldown.
	cooldown := time.Duration(policy.Spec.CooldownMinutes) * time.Minute
	if policy.Status.LastExpansionTime != nil {
		if since := time.Since(policy.Status.LastExpansionTime.Time); since < cooldown {
			wait := cooldown - since
			msg := fmt.Sprintf("trigger hit but in cooldown, %s remaining", wait.Round(time.Second))
			if err := r.update(ctx, &policy, "Cooldown", "Cooldown", msg, metav1.ConditionTrue); err != nil {
				return requeue, err
			}
			if wait < reconcileEvery {
				return ctrl.Result{RequeueAfter: wait}, nil
			}
			return requeue, nil
		}
	}

	// 5. Compute the new max, capped at the hard ceiling.
	newMax := pool.MaxNodes + policy.Spec.ExpandBy
	if newMax > policy.Spec.MaxNodes {
		newMax = policy.Spec.MaxNodes
	}
	if newMax <= pool.MaxNodes {
		msg := fmt.Sprintf("pool max %d is at/above ceiling %d; not expanding", pool.MaxNodes, policy.Spec.MaxNodes)
		return requeue, r.update(ctx, &policy, "AtCeiling", "AtCeiling", msg, metav1.ConditionTrue)
	}

	// 6. Expand.
	if err := r.DO.SetMaxNodes(ctx, pool.ClusterID, pool.PoolID, pool.Name, pool.MinNodes, newMax); err != nil {
		l.Error(err, "expanding pool max")
		return requeue, r.fail(ctx, &policy, "ExpandFailed", err.Error())
	}

	now := metav1.Now()
	policy.Status.LastExpansionTime = &now
	policy.Status.CurrentMaxNodes = newMax
	msg := fmt.Sprintf("raised pool autoscale max %d -> %d (ceiling %d)", pool.MaxNodes, newMax, policy.Spec.MaxNodes)
	l.Info("expanded", "from", pool.MaxNodes, "to", newMax)
	return requeue, r.update(ctx, &policy, "Expanded", "Expanded", msg, metav1.ConditionTrue)
}

// update writes phase/message/condition to status.
func (r *DoksCapacityOperatorReconciler) update(
	ctx context.Context, p *platformv1alpha1.DoksCapacityOperator,
	phase, reason, msg string, ready metav1.ConditionStatus,
) error {
	p.Status.Phase = phase
	p.Status.Message = msg
	meta.SetStatusCondition(&p.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             ready,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: p.Generation,
	})
	return r.Status().Update(ctx, p)
}

// fail is a convenience for the error phase.
func (r *DoksCapacityOperatorReconciler) fail(ctx context.Context, p *platformv1alpha1.DoksCapacityOperator, reason, msg string) error {
	return r.update(ctx, p, "Error", reason, msg, metav1.ConditionFalse)
}

// SetupWithManager wires the controller into the manager.
func (r *DoksCapacityOperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&platformv1alpha1.DoksCapacityOperator{}).
		Complete(r)
}
