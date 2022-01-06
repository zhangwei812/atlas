package config

import (
	"gopkg.in/urfave/cli.v1"
)

var (
	// Flags needed by abigen
	KeyFlag = cli.StringFlag{
		Name:  "key",
		Usage: "Private key file path",
		Value: "",
	}
	KeyStoreFlag = cli.StringFlag{
		Name:  "keystore",
		Usage: "Keystore file path",
	}
	PasswordFlag = cli.StringFlag{
		Name:  "password",
		Usage: "Keystore file`s password",
	}

	NamePrefixFlag = cli.StringFlag{
		Name:  "namePrefix",
		Usage: "Keystore file`s password",
	}
	CommissionFlag = cli.Int64Flag{
		Name:  "commission",
		Usage: "register validator param",
	}
	LesserFlag = cli.StringFlag{
		Name: "Lesser",
		Usage: "The validator receiving fewer votes than the validator for which the vote was revoked," +
			"or 0 if that validator has the fewest votes of any validator validator",
	}
	GreaterFlag = cli.StringFlag{
		Name: "Greater",
		Usage: "Greater The validator receiving more votes than the validator for which the vote was revoked," +
			"or 0 if that validator has the most votes of any validator validator.",
	}
	VoteNumFlag = cli.Int64Flag{
		Name:  "VoteNum",
		Usage: "The amount of gold to use to vote",
	}
	TopNumFlag = cli.Int64Flag{
		Name:  "topNum",
		Usage: "topNum of group`s member",
	}
	VerbosityFlag = cli.Int64Flag{
		Name:  "Verbosity",
		Usage: "Verbosity of log level",
	}

	RPCListenAddrFlag = cli.StringFlag{
		Name:  "rpcaddr",
		Usage: "HTTP-RPC server listening interface",
		Value: "localhost",
	}
	RPCPortFlag = cli.IntFlag{
		Name:  "rpcport",
		Usage: "HTTP-RPC server listening port",
		Value: 8545,
	}
	ValueFlag = cli.Uint64Flag{
		Name:  "value",
		Usage: "value units one eth",
		Value: 0,
	}

	TargetAddressFlag = cli.StringFlag{
		Name:  "target",
		Usage: "Transfer address",
		Value: "",
	}
)
