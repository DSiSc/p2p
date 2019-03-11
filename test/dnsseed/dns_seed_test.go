package main

import (
	"flag"
	"fmt"
	"github.com/DSiSc/p2p/tools/statistics/client"
	"os"
	"time"
)

func main() {
	var server string
	var nodeCount int
	flagSet := flag.NewFlagSet("dnsseed-test", flag.ExitOnError)
	flagSet.StringVar(&server, "server", "localhost:8080", "p2p debug server address")
	flagSet.IntVar(&nodeCount, "nodes", 1, "Number of the peer")
	flagSet.Usage = func() {
		fmt.Println(`Justitia blockchain p2p test tool.

Usage:
	dnsseed-test [-server localhost:8080] [-nodes 8]

Examples:
	dnsseed-test -nodes 8`)
		fmt.Println("Flags:")
		flagSet.PrintDefaults()
	}
	flagSet.Parse(os.Args[1:])

	statisticsClient := client.NewStatisticsClient(server)

	// get topo info
	topos, err := statisticsClient.GetTopos()
	if err != nil {
		fmt.Printf("Failed to get topo info from server, as: %v\n", err)
		os.Exit(1)
	}

	//calculate the reachability
	reachability := client.TopoReachbility(topos)
	if reachability < nodeCount {
		fmt.Printf("The net reachability is %d, less than nodes count： %d\n", reachability, nodeCount)
		os.Exit(1)
	}

	msgs, err := statisticsClient.GetAllReportMessage()
	if err != nil {
		fmt.Printf("Failed to get messages info from server, as: %v\n", err)
	}
	msgStr := make([]string, 0)
	for msg, routes := range msgs {
		if len(routes) < nodeCount {
			fmt.Printf("The rate of message %s's coverage is %d, less than nodes count： %d\n", msg, len(routes), nodeCount)
			os.Exit(1)
		}
		msgStr = append(msgStr, msg)
	}

	checkLongestBroadcastTime(msgStr, statisticsClient)
}

// check the longest broadcast time
func checkLongestBroadcastTime(msgs []string, statisticsClient *client.StatisticsClient) {
	for _, tx := range msgs {
		msgReport, err := statisticsClient.GetReportMessage(tx)
		if err != nil {
			fmt.Printf("failed to get tx %s's report info, as: %v", tx, err)
			os.Exit(1)
		}
		var timeStart, timeEnd *time.Time
		for _, report := range msgReport {
			if timeStart == nil || report.Time.Before(*timeStart) {
				timeStart = &report.Time
			}
			if timeEnd == nil || report.Time.After(*timeEnd) {
				timeEnd = &report.Time
			}
		}
		fmt.Printf("Tx %s broadcast time is %v\n", tx, timeEnd.Sub(*timeStart))
	}
}
