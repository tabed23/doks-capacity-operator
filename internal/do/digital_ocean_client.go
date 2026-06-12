package do


import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// Pool is a flattened view of a DOKS node pool.
type Pool struct {
	ClusterID string
	PoolID    string
	Name      string
	Count     int // live node count
	AutoScale bool
	MinNodes  int
	MaxNodes  int // the pool's autoscale max (the value we raise)
}

// Client is a thin wrapper around godo.
type Client struct {
	g *godo.Client
}

// New builds a client from a DigitalOcean API token.
func New(token string) *Client {
	return &Client{g: godo.NewFromToken(token)}
}

// ResolvePool returns the pool identified by poolID (preferred) or poolName.
//
//   - If clusterID is provided, it fetches the pool directly (one API call).
//   - Otherwise it lists the clusters visible to the token and matches the pool,
//     learning the cluster ID for you. PoolID matches are unambiguous; PoolName
//     matches error out if more than one cluster has a pool with that name.
func (c *Client) ResolvePool(ctx context.Context, clusterID, poolID, poolName string) (*Pool, error) {
	if poolID == "" && poolName == "" {
		return nil, fmt.Errorf("either poolID or poolName must be set")
	}

	// Fast path: caller already knows both IDs.
	if clusterID != "" && poolID != "" {
		np, _, err := c.g.Kubernetes.GetNodePool(ctx, clusterID, poolID)
		if err != nil {
			return nil, fmt.Errorf("get node pool %s/%s: %w", clusterID, poolID, err)
		}
		return toPool(clusterID, np), nil
	}

	// Discovery path: scan clusters and match.
	clusters, err := c.listAllClusters(ctx)
	if err != nil {
		return nil, err
	}

	var matches []*Pool
	for _, cl := range clusters {
		if clusterID != "" && cl.ID != clusterID {
			continue
		}
		for _, np := range cl.NodePools {
			matched := (poolID != "" && np.ID == poolID) ||
				(poolID == "" && poolName != "" && np.Name == poolName)
			if matched {
				matches = append(matches, toPool(cl.ID, np))
			}
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no node pool found (poolID=%q poolName=%q clusterID=%q)", poolID, poolName, clusterID)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("ambiguous: %d pools match poolName=%q across clusters; set poolID or clusterID to disambiguate", len(matches), poolName)
	}
}

// SetMaxNodes raises the pool's autoscale max. It keeps autoscaling enabled and
// preserves the existing min. Name is included because the DO update endpoint
// expects it.
func (c *Client) SetMaxNodes(ctx context.Context, clusterID, poolID, name string, minNodes, maxNodes int) error {
	autoScale := true
	req := &godo.KubernetesNodePoolUpdateRequest{
		Name:      name,
		AutoScale: &autoScale,
		MinNodes:  &minNodes,
		MaxNodes:  &maxNodes,
	}
	if _, _, err := c.g.Kubernetes.UpdateNodePool(ctx, clusterID, poolID, req); err != nil {
		return fmt.Errorf("update node pool %s/%s: %w", clusterID, poolID, err)
	}
	return nil
}

func (c *Client) listAllClusters(ctx context.Context) ([]*godo.KubernetesCluster, error) {
	var out []*godo.KubernetesCluster
	opt := &godo.ListOptions{Page: 1, PerPage: 200}
	for {
		clusters, resp, err := c.g.Kubernetes.List(ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("list clusters: %w", err)
		}
		out = append(out, clusters...)
		if resp == nil || resp.Links == nil || resp.Links.IsLastPage() {
			break
		}
		page, err := resp.Links.CurrentPage()
		if err != nil {
			return nil, err
		}
		opt.Page = page + 1
	}
	return out, nil
}

func toPool(clusterID string, np *godo.KubernetesNodePool) *Pool {
	// np.Count is the configured count; the live node count is len(np.Nodes).
	// Prefer the live count when nodes are populated.
	count := np.Count
	if live := len(np.Nodes); live > 0 {
		count = live
	}
	return &Pool{
		ClusterID: clusterID,
		PoolID:    np.ID,
		Name:      np.Name,
		Count:     count,
		AutoScale: np.AutoScale,
		MinNodes:  np.MinNodes,
		MaxNodes:  np.MaxNodes,
	}
}