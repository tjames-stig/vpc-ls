// Copyright 2021 Travis James - reference license in top level of project

package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"sync"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type lsvpcConfig struct {
	noColor    bool
	noSpace    bool
	allRegions bool
}

var Config lsvpcConfig

func populateVPC(region string) (map[string]VPC, error) {
	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
			Config: aws.Config{
				Region: aws.String(region),
			},
		},
	))

	svc := ec2.New(sess)
	var data RecievedData
	vpcs := make(map[string]VPC)

	data.wg.Add(12)
	go getVpcs(svc, &data)
	go getSubnets(svc, &data)
	go getInstances(svc, &data)
	go getNatGatways(svc, &data)
	go getRouteTables(svc, &data)
	go getInternetGateways(svc, &data)
	go getEgressOnlyInternetGateways(svc, &data)
	go getVPNGateways(svc, &data)
	go getTransitGatewayVpcAttachments(svc, &data)
	go getVpcPeeringConnections(svc, &data)
	go getNetworkInterfaces(svc, &data)
	go getVpcEndpoints(svc, &data)

	data.wg.Wait()

	// This isn't exhaustive error reporting, but all we really care about is if anything failed at all
	if data.Error != nil {
		return map[string]VPC{}, fmt.Errorf("failed to populate VPCs: %v", data.Error.Error())
	}

	mapVpcs(vpcs, data.Vpcs)
	mapSubnets(vpcs, data.Subnets)
	mapInstances(vpcs, data.Instances)
	err := instantiateVolumes(svc, vpcs)
	if err != nil {
		return map[string]VPC{}, fmt.Errorf("failed to populate VPCs: %v", err.Error())
	}
	mapNatGateways(vpcs, data.NatGateways)
	mapRouteTables(vpcs, data.RouteTables)
	mapInternetGateways(vpcs, data.InternetGateways)
	mapEgressOnlyInternetGateways(vpcs, data.EOInternetGateways)
	mapVPNGateways(vpcs, data.VPNGateways)
	mapTransitGatewayVpcAttachments(vpcs, data.TransitGateways)
	mapVpcPeeringConnections(vpcs, data.PeeringConnections)
	mapNetworkInterfaces(vpcs, data.NetworkInterfaces)
	mapVpcEndpoints(vpcs, data.VPCEndpoints)

	return vpcs, nil
}

func getRegionData(fullData map[string]RegionData, region string, wg *sync.WaitGroup, mu *sync.Mutex) {
	defer wg.Done()
	vpcs, err := populateVPC(region)
	if err != nil {
		return
	}
	mu.Lock()
	fullData[region] = RegionData{
		VPCs: vpcs,
	}
	mu.Unlock()
}

func doAllRegions() {
	var wg sync.WaitGroup

	regions := getRegions()

	fullData := make(map[string]RegionData)
	mu := sync.Mutex{}

	for _, region := range regions {
		wg.Add(1)
		go getRegionData(fullData, region, &wg, &mu)
	}

	wg.Wait()

	for region, vpcs := range fullData {
		fmt.Printf("===%v===\n", region)
		printVPCs(vpcs.VPCs)
	}

}

func doDefaultRegion() {
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}))

	currentRegion := aws.StringValue(sess.Config.Region)
	vpcs, err := populateVPC(currentRegion)
	if err != nil {
		panic("populateVPC failed")
	}

	printVPCs(vpcs)
}

func init() {
	flag.BoolVar(&Config.noColor, "nocolor", false, "Suppresses color printing of listing")
	flag.BoolVar(&Config.noSpace, "nospace", false, "Supresses line-spacing of items")
	flag.BoolVar(&Config.allRegions, "allregions", false, "Fetches and prints data on all regions")
	flag.BoolVar(&Config.allRegions, "all", false, "Fetches and prints data on all regions")
	flag.BoolVar(&Config.allRegions, "a", false, "Fetches and prints data on all regions")
}

func stdoutIsPipe() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		panic("Failed to stat stdout")
	}

	mode := info.Mode()
	return mode&fs.ModeNamedPipe != 0
}

func main() {
	flag.Parse()

	if stdoutIsPipe() {
		Config.noColor = true
	}

	if Config.allRegions {
		doAllRegions()
	} else {
		doDefaultRegion()
	}
}