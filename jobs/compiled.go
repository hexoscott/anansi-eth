package jobs

type ForgeCompiled struct {
	Abi []struct {
		Type            string `json:"type"`
		Inputs          []any  `json:"inputs"`
		StateMutability string `json:"stateMutability,omitempty"`
		Name            string `json:"name,omitempty"`
		Outputs         []struct {
			Name         string `json:"name"`
			Type         string `json:"type"`
			InternalType string `json:"internalType"`
		} `json:"outputs,omitempty"`
		Anonymous bool `json:"anonymous,omitempty"`
	} `json:"abi"`
	Bytecode struct {
		Object         string `json:"object"`
		SourceMap      string `json:"sourceMap"`
		LinkReferences struct {
		} `json:"linkReferences"`
	} `json:"bytecode"`
	DeployedBytecode struct {
		Object         string `json:"object"`
		SourceMap      string `json:"sourceMap"`
		LinkReferences struct {
		} `json:"linkReferences"`
	} `json:"deployedBytecode"`
	MethodIdentifiers struct {
		AllowanceAddressAddress           string `json:"allowance(address,address)"`
		ApproveAddressUint256             string `json:"approve(address,uint256)"`
		BalanceOfAddress                  string `json:"balanceOf(address)"`
		Decimals                          string `json:"decimals()"`
		MintUint256                       string `json:"mint(uint256)"`
		Name                              string `json:"name()"`
		Symbol                            string `json:"symbol()"`
		TotalSupply                       string `json:"totalSupply()"`
		TransferAddressUint256            string `json:"transfer(address,uint256)"`
		TransferFromAddressAddressUint256 string `json:"transferFrom(address,address,uint256)"`
	} `json:"methodIdentifiers"`
	RawMetadata string `json:"rawMetadata"`
	Metadata    struct {
		Compiler struct {
			Version string `json:"version"`
		} `json:"compiler"`
		Language string `json:"language"`
		Output   struct {
			Abi []struct {
				Inputs          []any  `json:"inputs"`
				StateMutability string `json:"stateMutability,omitempty"`
				Type            string `json:"type"`
				Name            string `json:"name,omitempty"`
				Anonymous       bool   `json:"anonymous,omitempty"`
				Outputs         []struct {
					InternalType string `json:"internalType"`
					Name         string `json:"name"`
					Type         string `json:"type"`
				} `json:"outputs,omitempty"`
			} `json:"abi"`
			Devdoc struct {
				Kind    string `json:"kind"`
				Methods struct {
					AllowanceAddressAddress struct {
						Details string `json:"details"`
					} `json:"allowance(address,address)"`
					ApproveAddressUint256 struct {
						Details string `json:"details"`
					} `json:"approve(address,uint256)"`
					BalanceOfAddress struct {
						Details string `json:"details"`
					} `json:"balanceOf(address)"`
					Decimals struct {
						Details string `json:"details"`
					} `json:"decimals()"`
					Name struct {
						Details string `json:"details"`
					} `json:"name()"`
					Symbol struct {
						Details string `json:"details"`
					} `json:"symbol()"`
					TotalSupply struct {
						Details string `json:"details"`
					} `json:"totalSupply()"`
					TransferAddressUint256 struct {
						Details string `json:"details"`
					} `json:"transfer(address,uint256)"`
					TransferFromAddressAddressUint256 struct {
						Details string `json:"details"`
					} `json:"transferFrom(address,address,uint256)"`
				} `json:"methods"`
				Version int `json:"version"`
			} `json:"devdoc"`
			Userdoc struct {
				Kind    string `json:"kind"`
				Methods struct {
				} `json:"methods"`
				Version int `json:"version"`
			} `json:"userdoc"`
		} `json:"output"`
		Settings struct {
			Remappings []string `json:"remappings"`
			Optimizer  struct {
				Enabled bool `json:"enabled"`
				Runs    int  `json:"runs"`
			} `json:"optimizer"`
			Metadata struct {
				BytecodeHash string `json:"bytecodeHash"`
			} `json:"metadata"`
			CompilationTarget struct {
				SrcSimpleERC20Sol string `json:"src/SimpleERC20.sol"`
			} `json:"compilationTarget"`
			EvmVersion string `json:"evmVersion"`
			Libraries  struct {
			} `json:"libraries"`
		} `json:"settings"`
		Sources struct {
			LibOpenzeppelinContractsContractsInterfacesDraftIERC6093Sol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"lib/openzeppelin-contracts/contracts/interfaces/draft-IERC6093.sol"`
			LibOpenzeppelinContractsContractsTokenERC20ERC20Sol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"lib/openzeppelin-contracts/contracts/token/ERC20/ERC20.sol"`
			LibOpenzeppelinContractsContractsTokenERC20IERC20Sol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"lib/openzeppelin-contracts/contracts/token/ERC20/IERC20.sol"`
			LibOpenzeppelinContractsContractsTokenERC20ExtensionsIERC20MetadataSol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"lib/openzeppelin-contracts/contracts/token/ERC20/extensions/IERC20Metadata.sol"`
			LibOpenzeppelinContractsContractsUtilsContextSol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"lib/openzeppelin-contracts/contracts/utils/Context.sol"`
			SrcSimpleERC20Sol struct {
				Keccak256 string   `json:"keccak256"`
				Urls      []string `json:"urls"`
				License   string   `json:"license"`
			} `json:"src/SimpleERC20.sol"`
		} `json:"sources"`
		Version int `json:"version"`
	} `json:"metadata"`
	ID int `json:"id"`
}
