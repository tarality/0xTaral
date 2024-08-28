package core

import (
	"bytes"
	"errors"
	"math"
	"math/big"
	"sync"

	"github.com/tarality/0xTaral/messages/proto"
)

var (
	errVotingPowerNotCorrect = errors.New("total voting power is zero or less")
)

// ValidatorBackend defines interface that has GetVotingPower
type ValidatorBackend interface {
	// GetVotingPowers returns map of validators addresses and their voting powers for the specified height.
	GetVotingPowers(height uint64) (map[string]*big.Int, error)
}

// ValidatorManager keeps voting power and other information about validators
type ValidatorManager struct {
	vpLock *sync.RWMutex

	// quorumSize represents quorum for the height specified in the current View
	quorumSize *big.Int

	// validatorsVotingPower is a map of the validator addresses on their voting power for
	// the height specified in the current View
	validatorsVotingPower map[string]*big.Int

	backend ValidatorBackend

	log Logger
}

// NewValidatorManager creates new ValidatorManager
func NewValidatorManager(backend ValidatorBackend, log Logger) *ValidatorManager {
	return &ValidatorManager{
		quorumSize:            big.NewInt(0),
		backend:               backend,
		validatorsVotingPower: nil,
		log:                   log,
		vpLock:                &sync.RWMutex{},
	}
}

// Init sets voting power and quorum size
func (vm *ValidatorManager) Init(height uint64) error {
	validatorsVotingPower, err := vm.backend.GetVotingPowers(height)
	if err != nil {
		return err
	}

	return vm.setCurrentVotingPower(validatorsVotingPower)
}

// setCurrentVotingPower sets the current total voting power and quorum size
// based on current validators voting power
func (vm *ValidatorManager) setCurrentVotingPower(validatorsVotingPower map[string]*big.Int) error {
	vm.vpLock.Lock()
	defer vm.vpLock.Unlock()

	totalVotingPower := calculateTotalVotingPower(validatorsVotingPower)
	if totalVotingPower.Cmp(big.NewInt(0)) <= 0 {
		return errVotingPowerNotCorrect
	}

	vm.validatorsVotingPower = validatorsVotingPower
	vm.quorumSize = calculateQuorum(totalVotingPower)

	return nil
}

// HasQuorum provides information on whether messages have reached the quorum
func (vm *ValidatorManager) HasQuorum(sendersAddrs map[string]struct{}) bool {
	vm.vpLock.RLock()
	defer vm.vpLock.RUnlock()

	// if not initialized correctly return false
	if vm.validatorsVotingPower == nil {
		return false
	}

	messageVotePower := big.NewInt(0)

	for from := range sendersAddrs {
		if vote, ok := vm.validatorsVotingPower[from]; ok {
			messageVotePower.Add(messageVotePower, vote)
		}
	}

	// aggVotingPower >= (2 * totalVotingPower / 3) + 1
	return messageVotePower.Cmp(vm.quorumSize) >= 0
}

// HasPrepareQuorum provides information on whether prepared messages have reached the quorum
func (vm *ValidatorManager) HasPrepareQuorum(stateName stateType, proposalMessage *proto.Message,
	msgs []*proto.Message) bool {
	if proposalMessage == nil {
		// If the state is in prepare phase, the proposal must be set. Otherwise, just return false since
		// this is a valid scenario e.g. proposal msg is received before prepare msg for the same view
		if stateName == prepare {
			vm.log.Error("HasPrepareQuorum - proposalMessage is not set")
		}

		return false
	}

	proposerAddress := proposalMessage.From
	sendersAddressesMap := map[string]struct{}{
		string(proposerAddress): {},
	}

	for _, message := range msgs {
		if bytes.Equal(message.From, proposerAddress) {
			vm.log.Error("HasPrepareQuorum - proposer is among signers but it is not expected to be")

			return false
		}

		sendersAddressesMap[string(message.From)] = struct{}{}
	}

	return vm.HasQuorum(sendersAddressesMap)
}

// calculateQuorum calculates quorum size which is FLOOR(2 * totalVotingPower / 3) + 1
//func calculateQuorum(totalVotingPower *big.Int) *big.Int {
//	quorum := new(big.Int).Mul(totalVotingPower, big.NewInt(2))

// this will floor the (2 * totalVotingPower/3) and add 1
//	return quorum.Div(quorum, big.NewInt(3)).Add(quorum, big.NewInt(1))
//}

// This below function accept 60%-40% mean 60% must be active , protocol can tollerate 40% of validators offlibe

func calculateQuorum(totalVotingPower *big.Int) *big.Int {
	// Multiply totalVotingPower by 6 and then divide by 10
	// totalVotingPower := big.NewInt(4)
	quorumPercentage := 0.6 // 60%

	// Convert total voting power to big.Float
	totalVotingPowerFloat := new(big.Float).SetInt(totalVotingPower)

	// Convert quorum percentage to big.Float
	quorumPercentageFloat := big.NewFloat(quorumPercentage)

	// Perform the calculation: quorumPercentage * totalVotingPower
	quorumFloat := new(big.Float).Mul(quorumPercentageFloat, totalVotingPowerFloat)

	// Convert the result to a float64
	quorumFloat64, _ := quorumFloat.Float64()

	// Apply math.Ceil to the result
	quorumCeilFloat64 := (math.Ceil(quorumFloat64))
	quorumCeilBigFloat := new(big.Float).SetFloat64(quorumCeilFloat64)
	quorumBigInt := new(big.Int)
	quorumCeilBigFloat.Int(quorumBigInt)

	// If no remainder, return the quorum as it is
	return quorumBigInt
}

func calculateTotalVotingPower(validatorsVotingPower map[string]*big.Int) *big.Int {
	totalVotingPower := big.NewInt(0)
	for _, validatorVotingPower := range validatorsVotingPower {
		totalVotingPower.Add(totalVotingPower, validatorVotingPower)
	}

	return totalVotingPower
}

// convertMessageToAddressSet converts messages slice to addresses map
func convertMessageToAddressSet(messages []*proto.Message) map[string]struct{} {
	result := make(map[string]struct{}, len(messages))

	for _, x := range messages {
		result[string(x.From)] = struct{}{}
	}

	return result
}
