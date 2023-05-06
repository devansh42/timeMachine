package dht

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/cespare/xxhash/v2"
)

type SlotState string
type SlotID int
type NodeID string

var (
	Leader   SlotState = "leader"
	Follower SlotState = "follower"
)

type SlotInfo struct {
	NodeID    NodeID
	SlotState SlotState
}

type SlotAndNode struct {
	SlotID SlotID
	NodeID NodeID
}

type dht struct {

	// maintains the location of all slots slotid vs nodeid
	slotVsNodes map[SlotID]*SlotInfo

	mu sync.RWMutex
}

var _ DHT = &dht{}

func Create() *dht {
	return &dht{}
}

// Creates a new distributed hash table from the inputs.
// Should be called only from bootstrap mode or while creating a new cluster
func (d *dht) Initialise(slotCountperNode int, nodes []string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.slotVsNodes) > 0 {
		return ErrAlreadyInitialised
	}

	// nodeVsSlot contains the mapping of nodeID to slotID.
	nodeVsSlot := make(map[NodeID][]SlotID)

	nodeCount := len(nodes)
	slotCount := slotCountperNode * nodeCount
	d.slotVsNodes = make(map[SlotID]*SlotInfo)

	// The distribution below makes sure the slots are
	// assigned equally in a round robin manner
	distribution := make([]int, nodeCount)
	for i := 0; i < slotCount; i++ {
		distribution[i%nodeCount]++
	}

	slotNumber := 0
	for i := 0; i < len(distribution); i++ { // For each of the node
		for j := 0; j < distribution[i]; j++ { // For the distribution count assigned to that node
			nodeID := NodeID(nodes[i])
			slotID := SlotID(slotNumber)

			d.slotVsNodes[slotID] = &SlotInfo{
				NodeID: nodeID,
			}
			nodeVsSlot[nodeID] = append(nodeVsSlot[nodeID], slotID)
			slotNumber++
		}
	}

	// Assign leaders in a round robin manner
	for i := 0; i < slotCountperNode; i++ {
		for nodeID := range nodeVsSlot {
			slotID := nodeVsSlot[nodeID][i]
			slotInfo := d.slotVsNodes[slotID]
			if slotInfo.SlotState != "" {
				continue
			}
			slotInfo.SlotState = Leader

			replicaSlotInfo := d.slotVsNodes[d.replicaSlot(slotID)]
			replicaSlotInfo.SlotState = Follower
		}
	}

	by, _ := json.Marshal(d.slotVsNodes)
	fmt.Printf("Slot Info: %s\n", string(by))

	return nil
}

// Loads data from a already existing configuration.
// This must be called only after confirmation from the master
func (d *dht) Load(data []byte) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.slotVsNodes) > 0 {
		return ErrAlreadyInitialised
	}

	slotVsNodes := make(map[SlotID]*SlotInfo)
	return json.Unmarshal(data, &slotVsNodes)
}

// Snapshot returns the node vs slot ids map in json format
func (d *dht) Snapshot() (data []byte, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	return json.Marshal(&d.slotVsNodes)
}

// Returns the location of the leader and follower slots and their corresponding nodes
func (d *dht) GetLocation(key string) (leader *SlotAndNode, follower *SlotAndNode, err error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.slotVsNodes) == 0 {
		return nil, nil, ErrNotInitialised
	}

	slot1 := SlotID(d.hashSlot(key))
	node1 := d.slotVsNodes[slot1]
	sn1 := &SlotAndNode{
		SlotID: slot1,
		NodeID: node1.NodeID,
	}

	slot2 := d.replicaSlot(slot1)
	node2 := d.slotVsNodes[slot2]
	sn2 := &SlotAndNode{
		SlotID: slot2,
		NodeID: node2.NodeID,
	}

	if node1.SlotState == Leader {
		return sn1, sn2, nil // sn1 is the leader
	} else {
		return sn2, sn1, nil // sn2 is the leader
	}
}

func (d *dht) hashSlot(key string) int {
	slotCount := uint64(len(d.slotVsNodes))
	hashValue := xxhash.Sum64([]byte(key))
	return int(hashValue % slotCount)
}

func (d *dht) replicaSlot(location1 SlotID) SlotID {
	slotCount := len(d.slotVsNodes)
	return SlotID((int(location1) + slotCount/2) % slotCount)
}
