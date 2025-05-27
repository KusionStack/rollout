// Copyright 2023 The KusionStack Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cli

import (
	"flag"
	"fmt"
	"os"

	_ "sigs.k8s.io/controller-runtime/pkg/client/config" // init kubeconfig flag

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/term"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog/v2"
)

func AddFlagsAndUsage(cmd *cobra.Command, namedFlagSets *cliflag.NamedFlagSets) {
	// add version flag
	global := namedFlagSets.FlagSet("global")
	verflag.AddFlags(global)
	// add go flags, e.g kubeconfig
	AddKubeconfigFlag(global)
	// add help
	global.BoolP("help", "h", false, fmt.Sprintf("help for %s", cmd.Name()))

	fs := cmd.Flags()
	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}

	// set usage
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), *namedFlagSets, cols)
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), *namedFlagSets, cols)
	})
}

// addKlogFlags adds flags from k8s.io/klog
func AddKlogFlags(fs *pflag.FlagSet) {
	local := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	klog.InitFlags(local)
	normalizeFunc := fs.GetNormalizeFunc()
	local.VisitAll(func(fl *flag.Flag) {
		fl.Name = string(normalizeFunc(fs, fl.Name))
		fs.AddGoFlag(fl)
	})
}

// PrintFlags logs the flags in the flagset
func PrintFlags(logger logr.Logger, flags *pflag.FlagSet) {
	flags.VisitAll(func(flag *pflag.Flag) {
		logger.V(1).Info(fmt.Sprintf("FLAG: --%s=%s", flag.Name, flag.Value))
	})
}

func AddKubeconfigFlag(fs *pflag.FlagSet) {
	f := flag.CommandLine.Lookup("kubeconfig")
	if f == nil {
		return
	}
	fs.AddGoFlag(f)
}
