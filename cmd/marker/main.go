package main

import (
	"fmt"
	"gopkg.in/urfave/cli.v1"
	"os"
	"sort"
	"strconv"

	"github.com/ethereum/go-ethereum/log"

	"github.com/mapprotocol/atlas/cmd/marker/config"
	"github.com/mapprotocol/atlas/cmd/marker/geneisis"
)

var (
	// The app that holds all commands and flags.
	app   *cli.App
	Flags = []cli.Flag{
		config.KeyFlag,
		config.KeyStoreFlag,
		config.RPCListenAddrFlag,
		config.RPCPortFlag,
		config.ValueFlag,
		config.PasswordFlag,
		config.CommissionFlag,
		config.LesserFlag,
		config.GreaterFlag,
		config.VoteNumFlag,
		config.TopNumFlag,
		config.TargetAddressFlag,
	}

	validatorCommand = cli.Command{
		Name:  "validator",
		Usage: "validator commands",
		Subcommands: []cli.Command{
			createAccountCommand,
			lockedMAPCommand,
			registerValidatorCommand,
			unlockedMAPCommand,
			relockMAPCommand,
			withdrawCommand,

			queryRegisteredValidatorSignersCommand,
			queryTopValidatorsCommand,
		},
		Flags: Flags,
	}
	voterCommand = cli.Command{
		Name:  "voter",
		Usage: "voter commands",
		Subcommands: []cli.Command{
			voteValidatorCommand,
			getValidatorEligibilityCommand,
			getTotalVotesForVCommand,
			getBalanceCommand,
			activateCommand,
			queryRegisteredValidatorSignersCommand,
			queryTopValidatorsCommand,
		},
		Flags: Flags,
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
		genesis.CreateGenesisCommand,
	}
	app.Flags = Flags
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

func MigrateFlags(hdl func(ctx *cli.Context, config *listener) error) func(*cli.Context) error {
	return func(ctx *cli.Context) error {
		for _, name := range ctx.FlagNames() {
			if ctx.IsSet(name) {
				ctx.GlobalSet(name, ctx.String(name))
			}
		}
		config := config.AssemblyConfig(ctx)
		err := startLogger(ctx, config)
		if err != nil {
			panic(err)
		}
		core := NewListener(ctx, config)
		writer := NewWriter(ctx, config)
		core.setWriter(writer)
		return hdl(ctx, core)
	}
}
func startLogger(ctx *cli.Context, config *config.Config) error {
	logger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	var lvl log.Lvl
	if lvlToInt, err := strconv.Atoi(config.Verbosity); err == nil {
		lvl = log.Lvl(lvlToInt)
	} else if lvl, err = log.LvlFromString(config.Verbosity); err != nil {
		return err
	}
	logger.Verbosity(lvl)
	log.Root().SetHandler(log.LvlFilterHandler(lvl, logger))

	return nil
}
