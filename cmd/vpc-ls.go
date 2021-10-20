package main

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

type VPC struct {
	Id        string
	IsDefault bool
	CidrBlock string
	RawVPC    *ec2.Vpc
	Subnets   map[string]Subnet
}

type Subnet struct {
	Id                 string
	CidrBlock          string
	AvailabilityZone   string
	AvailabilityZoneId string
	Public             bool
	RawSubnet          *ec2.Subnet
	EC2s               map[string]EC2
}

type EC2 struct {
	Id         string
	Type       string
	SubnetId   string
	VpcId      string
	State      string
	PublicIP   string
	PrivateIP  string
	Interfaces []NetworkInterface
	RawEc2     *ec2.Instance
}

type NetworkInterface struct {
	Id                  string
	PrivateIp           string
	MAC                 string
	DNS                 string
	RawNetworkInterface *ec2.InstanceNetworkInterface
}

func getVpcs(svc *ec2.EC2) (map[string]VPC, error) {
	vpclist := []*ec2.Vpc{}
	err := svc.DescribeVpcsPages(
		&ec2.DescribeVpcsInput{},
		func(page *ec2.DescribeVpcsOutput, lastPage bool) bool {
			vpclist = append(vpclist, page.Vpcs...)
			return !lastPage
		},
	)

	if err != nil {
		return map[string]VPC{}, fmt.Errorf("failed to get vpcs: %v", err.Error())
	}
	vpcs := map[string]VPC{}
	for _, v := range vpclist {
		vpc := VPC{
			Id:        *v.VpcId,
			IsDefault: *v.IsDefault,
			CidrBlock: *v.CidrBlock,
			RawVPC:    v,
			Subnets:   make(map[string]Subnet),
		}
		vpcs[*v.VpcId] = vpc
	}

	return vpcs, nil

}

func getSubnets(svc *ec2.EC2) ([]*ec2.Subnet, error) {
	subnets := []*ec2.Subnet{}
	err := svc.DescribeSubnetsPages(
		&ec2.DescribeSubnetsInput{},
		func(page *ec2.DescribeSubnetsOutput, lastPage bool) bool {
			subnets = append(subnets, page.Subnets...)
			return !lastPage
		},
	)

	if err != nil {
		return []*ec2.Subnet{}, fmt.Errorf("%v", err.Error())
	}

	return subnets, nil
}

func getInstances(svc *ec2.EC2) ([]*ec2.Reservation, error) {
	instances := []*ec2.Reservation{}
	err := svc.DescribeInstancesPages(
		&ec2.DescribeInstancesInput{},
		func(page *ec2.DescribeInstancesOutput, lastPage bool) bool {
			instances = append(instances, page.Reservations...)
			return !lastPage
		},
	)
	if err != nil {
		return []*ec2.Reservation{}, fmt.Errorf("failed: %v", err.Error())
	}

	return instances, nil
}

func getNetworkInterfaces(svc *ec2.EC2) ([]*ec2.NetworkInterface, error) {
	networkInterfaces := []*ec2.NetworkInterface{}
	err := svc.DescribeNetworkInterfacesPages(
		&ec2.DescribeNetworkInterfacesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("status"),
					Values: []*string{
						aws.String("in-use"),
					},
				},
			},
		},
		func(page *ec2.DescribeNetworkInterfacesOutput, lastPage bool) bool {
			networkInterfaces = append(networkInterfaces, page.NetworkInterfaces...)
			return !lastPage
		},
	)
	if err != nil {
		return []*ec2.NetworkInterface{}, err
	}

	return networkInterfaces, nil
}

func mapSubnets(vpcs map[string]VPC, subnets []*ec2.Subnet) {
	for _, v := range subnets {
		isPublic := *v.MapCustomerOwnedIpOnLaunch || *v.MapPublicIpOnLaunch

		vpcs[*v.VpcId].Subnets[*v.SubnetId] = Subnet{
			Id:                 *v.SubnetId,
			CidrBlock:          *v.CidrBlock,
			AvailabilityZone:   *v.AvailabilityZone,
			AvailabilityZoneId: *v.AvailabilityZoneId,
			RawSubnet:          v,
			Public:             isPublic,
			EC2s:               make(map[string]EC2),
		}

	}
}

func mapInstances(vpcs map[string]VPC, reservations []*ec2.Reservation) {
	for _, reservation := range reservations {
		for _, instance := range reservation.Instances {
			networkInterfaces := []NetworkInterface{}
			for _, networkInterface := range instance.NetworkInterfaces {
				networkInterfaces = append(networkInterfaces, NetworkInterface{
					Id:                  *networkInterface.NetworkInterfaceId,
					PrivateIp:           *networkInterface.PrivateIpAddress,
					MAC:                 *networkInterface.MacAddress,
					DNS:                 *networkInterface.PrivateDnsName,
					RawNetworkInterface: networkInterface,
				})
			}

			vpcs[*instance.VpcId].Subnets[*instance.SubnetId].EC2s[*instance.InstanceId] = EC2{
				Id:         *instance.InstanceId,
				Type:       *instance.InstanceType,
				SubnetId:   *instance.SubnetId,
				VpcId:      *instance.VpcId,
				State:      *instance.State.Name,
				PublicIP:   *instance.PublicIpAddress,
				PrivateIP:  *instance.PrivateIpAddress,
				Interfaces: networkInterfaces,
				RawEc2:     instance,
			}
		}
	}
}

func indent(num int) string {
	sb := strings.Builder{}

	for i := 0; i <= num; i++ {
		sb.WriteString(" ")
	}

	return sb.String()
}

func printVPCs(vpcs map[string]VPC) {
	for _, v := range vpcs {
		if v.IsDefault {
			fmt.Printf("(default) ")
		}
		fmt.Printf("%v --- %v\n", v.Id, v.CidrBlock)
		for _, w := range v.Subnets {
			public := "Private"
			if w.Public {
				public = "Public"
			}
			fmt.Printf("%s%v %v --- %v - %v\n", indent(4), w.Id, w.AvailabilityZone, w.CidrBlock, public)
			for _, x := range w.EC2s {
				fmt.Printf(
					"%s%v --- %v --- %v / %v\n",
					indent(8),
					x.Id,
					x.State,
					x.PublicIP,
					x.PrivateIP,
				)

				fmt.Printf("%sInterfaces:\n", indent(10))
				for _, y := range x.Interfaces {
					fmt.Printf("%s%v %v %v %v\n", indent(12), y.Id, y.MAC, y.PrivateIp, y.DNS)
				}
			}
		}
	}
}

func main() {
	sess := session.Must(session.NewSessionWithOptions(
		session.Options{
			SharedConfigState: session.SharedConfigEnable,
		},
	))

	svc := ec2.New(sess)

	vpcs, err := getVpcs(svc)
	if err != nil {
		fmt.Printf("failed to get vpcs: %v", err.Error())
	}

	subnets, _ := getSubnets(svc)
	mapSubnets(vpcs, subnets)
	instances, _ := getInstances(svc)
	mapInstances(vpcs, instances)

	printVPCs(vpcs)
}
