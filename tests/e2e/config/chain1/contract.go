package consts

import (
	"github.com/ethereum/go-ethereum/common"
)

const (
	IBCStoreAddress = "0x9B05f07FC9EF14f3b18c9612eB7e31CF12FDa068"
	IBCModuleAddress = "0xA05d3D10aB5aB40f5e7751411a4ff975e6Ecc97e"
	IBFT2ClientAddress = "0x2E94C9178569870655b5a12871a7FA9Aed8Bd5ef"
	SimpleTokenModuleAddress = "0x747296FC9d600e4Ce2156dE3aeE8aa75bf2E459a"
)

type contractConfig struct{}

var Contract contractConfig

func (contractConfig) GetIBCStoreAddress() common.Address {
	return common.HexToAddress(IBCStoreAddress)
}

func (contractConfig) GetIBCModuleAddress() common.Address {
	return common.HexToAddress(IBCModuleAddress)
}

func (contractConfig) GetIBFT2ClientAddress() common.Address {
	return common.HexToAddress(IBFT2ClientAddress)
}

func (contractConfig) GetSimpleTokenModuleAddress() common.Address {
	return common.HexToAddress(SimpleTokenModuleAddress)
}
