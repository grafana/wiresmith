package basic

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	rec "github.com/grafana/wiresmith/gen/basic/recursive/v1"
)

func TestLinkedList_ThreeNodes(t *testing.T) {
	msg := &rec.LinkedList{
		Value: 1,
		Next: &rec.LinkedList{
			Value: 2,
			Next: &rec.LinkedList{
				Value: 3,
			},
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, msg.Size(), len(b))

	dst := &rec.LinkedList{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, msg.Equal(dst))

	assert.Equal(t, int64(1), dst.Value)
	require.NotNil(t, dst.Next)
	assert.Equal(t, int64(2), dst.Next.Value)
	require.NotNil(t, dst.Next.Next)
	assert.Equal(t, int64(3), dst.Next.Next.Value)
	assert.Nil(t, dst.Next.Next.Next)
}

func TestLinkedList_SingleNode(t *testing.T) {
	msg := &rec.LinkedList{Value: 42}
	roundTrip(t, msg)
}

func TestLinkedList_NilNext(t *testing.T) {
	msg := &rec.LinkedList{}
	b := roundTrip(t, msg)

	dst := &rec.LinkedList{}
	require.NoError(t, dst.Unmarshal(b))
	assert.Nil(t, dst.Next)
	assert.Nil(t, dst.GetNext())
}

func TestTreeNode_DepthThree(t *testing.T) {
	msg := &rec.TreeNode{
		Label: "root",
		Value: 0,
		Children: []rec.TreeNode{
			{
				Label: "child1",
				Value: 1,
				Children: []rec.TreeNode{
					{Label: "grandchild1a", Value: 10},
					{Label: "grandchild1b", Value: 11},
				},
			},
			{
				Label: "child2",
				Value: 2,
			},
		},
	}
	roundTrip(t, msg)
}

func TestTreeNode_Leaf(t *testing.T) {
	roundTrip(t, &rec.TreeNode{Label: "leaf", Value: 99})
}

func TestNodeAB_MutualRecursion(t *testing.T) {
	msg := &rec.NodeA{
		Name: "a-root",
		Peer: &rec.NodeB{
			Id: 1,
			Parent: &rec.NodeA{
				Name: "a-inner",
			},
		},
		Peers: []rec.NodeB{
			{Id: 10},
			{Id: 20, Parent: &rec.NodeA{Name: "a-peer-parent"}},
		},
	}
	b, err := msg.Marshal()
	require.NoError(t, err)
	assert.Equal(t, msg.Size(), len(b))

	dst := &rec.NodeA{}
	require.NoError(t, dst.Unmarshal(b))
	assert.True(t, msg.Equal(dst))

	require.NotNil(t, dst.Peer)
	assert.Equal(t, int64(1), dst.Peer.Id)
	require.NotNil(t, dst.Peer.Parent)
	assert.Equal(t, "a-inner", dst.Peer.Parent.Name)
}

func TestLinkedList_Equal(t *testing.T) {
	a := &rec.LinkedList{Value: 1, Next: &rec.LinkedList{Value: 2}}
	b := &rec.LinkedList{Value: 1, Next: &rec.LinkedList{Value: 2}}
	assert.True(t, a.Equal(b))

	c := &rec.LinkedList{Value: 1, Next: &rec.LinkedList{Value: 3}}
	assert.False(t, a.Equal(c))

	d := &rec.LinkedList{Value: 1}
	assert.False(t, a.Equal(d))
}
