package main

import (
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"os"
	"sort"
)

var (
	// The app that holds all commands and flags.
	app *cli.App

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
	lesserFlag = cli.StringFlag{
		Name: "lesser",
		Usage: "The validator receiving fewer votes than the validator for which the vote was revoked," +
			"or 0 if that validator has the fewest votes of any validator validator",
	}
	greaterFlag = cli.StringFlag{
		Name: "greater",
		Usage: "greater The validator receiving more votes than the validator for which the vote was revoked," +
			"or 0 if that validator has the most votes of any validator validator.",
	}
	voteNumFlag = cli.Int64Flag{
		Name:  "voteNum",
		Usage: "The amount of gold to use to vote",
	}
	TopNumFlag = cli.Int64Flag{
		Name:  "topNum",
		Usage: "topNum of group`s member",
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

	ValidatorFlags = []cli.Flag{
		KeyFlag,
		KeyStoreFlag,
		RPCListenAddrFlag,
		RPCPortFlag,
		ValueFlag,
		PasswordFlag,
		CommissionFlag,
		lesserFlag,
		greaterFlag,
		voteNumFlag,
		TopNumFlag,
		TargetAddressFlag,
	}
)

func init() {
	app = cli.NewApp()
	app.Usage = "Atlas Marker Tool"
	app.Name = "marker"
	app.Version = "1.0.0"
	app.Copyright = "Copyright 2020-2021 The Atlas Authors"
	app.Action = MigrateFlags(registerValidator)
	app.CommandNotFound = func(ctx *cli.Context, cmd string) {
		fmt.Fprintf(os.Stderr, "No such command: %s\n", cmd)
		os.Exit(1)
	}
	// Add subcommands.
	app.Commands = []cli.Command{
		validatorCommand,
		voterCommand,
	}
	cli.CommandHelpTemplate = OriginCommandHelpTemplate
	sort.Sort(cli.CommandsByName(app.Commands))
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var OriginCommandHelpTemplate string = `{{.Name}}{{if .Subcommands}} command{{end}}{{if .Flags}} [command options]{{end}} [arguments...] {{if .Description}}{{.Description}} {{end}}{{if .Subcommands}} SUBCOMMANDS:     {{range .Subcommands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}     {{end}}{{end}}{{if .Flags}} OPTIONS: {{range $.Flags}}{{"\t"}}{{.}} {{end}} {{end}}`

func MigrateFlags(hdl func(ctx *cli.Context, config *Config) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		return hdl(ctx, AssemblyConfig(ctx))
	}
}
