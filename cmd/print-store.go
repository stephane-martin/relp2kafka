// Copyright © 2017 NAME HERE <EMAIL ADDRESS>
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

package cmd

import (
	"fmt"

	"github.com/inconshreveable/log15"
	"github.com/spf13/cobra"
	"github.com/stephane-martin/relp2kafka/conf"
	"github.com/stephane-martin/relp2kafka/consul"
	"github.com/stephane-martin/relp2kafka/store"
)

// printStoreCmd represents the printStore command
var printStoreCmd = &cobra.Command{
	Use:   "print-store",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("print-store called")

		var err error
		var c *conf.GConfig
		var st *store.MessageStore
		logger := log15.New()
		params := consul.ConnParams{Address: consulAddr, Datacenter: consulDC, Token: consulToken}

		c, _, err = conf.InitLoad(configDirName, params, consulPrefix, logger)
		if err != nil {
			fmt.Println("bleh")
			return
		}

		// prepare the message store
		st, err = store.NewStore(c, logger, testFlag)
		if err != nil {
			fmt.Println("Can't create the message Store", "error", err)
			return
		}

		messagesMap, readyMap, failedMap, sentMap := st.ReadAll()

		fmt.Println("Messages")
		for k, v := range messagesMap {
			fmt.Printf("%s %s\n", k, v)
		}
		fmt.Println()

		fmt.Println("Ready")
		for k, v := range readyMap {
			fmt.Printf("%s %s\n", k, v)
		}
		fmt.Println()

		fmt.Println("Failed")
		for k, v := range failedMap {
			fmt.Printf("%s %s\n", k, v)
		}
		fmt.Println()

		fmt.Println("Sent")
		for k, v := range sentMap {
			fmt.Printf("%s %s\n", k, v)
		}
		fmt.Println()

	},
}

func init() {
	RootCmd.AddCommand(printStoreCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// printStoreCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// printStoreCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
