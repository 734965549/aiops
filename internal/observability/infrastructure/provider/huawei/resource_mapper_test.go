package huawei

import (
	"testing"

	dcsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dcs/v2/model"
	ecsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/ecs/v2/model"
	eipmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/eip/v2/model"
	evsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/evs/v2/model"
	kafkamodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/kafka/v2/model"
	rdsmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rds/v3/model"
	rocketmqmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/rocketmq/v2/model"
	vpcmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/vpc/v2/model"
)

func TestMapECSServer(t *testing.T) {
	res := mapECSServer("cn-north-4", ecsmodel.ServerDetail{
		Id: "srv-1", Name: "web-01", Status: "ACTIVE",
	})
	if res.Type != "ecs" || res.Region != "cn-north-4" || res.ProviderRef != "srv-1" {
		t.Fatalf("unexpected ecs mapping: %+v", res)
	}
	if res.Labels["instance_id"] != "srv-1" {
		t.Fatalf("expected instance_id label, got %+v", res.Labels)
	}
}

func TestMapRDSInstance(t *testing.T) {
	res := mapRDSInstance("cn-north-4", rdsmodel.InstanceResponse{
		Id: "rds-1", Name: "db-main", Status: "ACTIVE",
	})
	if res.Type != "rds" || res.ProviderRef != "rds-1" || res.Name != "db-main" {
		t.Fatalf("unexpected rds mapping: %+v", res)
	}
}

func TestMapECSServerEnrichmentLabels(t *testing.T) {
	ip := "10.0.0.1"
	res := mapECSServer("cn-south-1", ecsmodel.ServerDetail{
		Id:     "srv-1",
		Name:   "web-01",
		Status: "ACTIVE",
		HostId: "host-abc",
		Addresses: map[string][]ecsmodel.ServerAddress{
			"vpc-aaa": {
				{Addr: ip, Version: "4"},
				{Addr: "::1", Version: "6"},
			},
		},
		Flavor: &ecsmodel.ServerFlavor{
			Id: "s6.large.2", Name: "s6.large.2", Vcpus: "2", Ram: "4096", Disk: "40",
		},
		Metadata:                map[string]string{"vpc_id": "vpc-aaa"},
		OSEXTAZavailabilityZone: "cn-south-1a",
	})
	if res.Labels["private_ip"] != ip {
		t.Fatalf("private_ip = %q, want %q", res.Labels["private_ip"], ip)
	}
	if res.Labels["flavor"] != "s6.large.2" {
		t.Fatalf("flavor = %q, want s6.large.2", res.Labels["flavor"])
	}
	if res.Labels["vpc_id"] != "vpc-aaa" {
		t.Fatalf("vpc_id = %q, want vpc-aaa", res.Labels["vpc_id"])
	}
	if res.Labels["az"] != "cn-south-1a" {
		t.Fatalf("az = %q, want cn-south-1a", res.Labels["az"])
	}
	if res.Labels["host_id"] != "host-abc" {
		t.Fatalf("host_id = %q, want host-abc", res.Labels["host_id"])
	}
}

func TestMapECSServerNoEnrichmentLabelsWhenAbsent(t *testing.T) {
	res := mapECSServer("cn-south-1", ecsmodel.ServerDetail{
		Id: "srv-2", Name: "web-02", Status: "ACTIVE",
	})
	if _, ok := res.Labels["private_ip"]; ok {
		t.Fatal("expected no private_ip when Addresses empty")
	}
	if _, ok := res.Labels["flavor"]; ok {
		t.Fatal("expected no flavor when Flavor nil")
	}
	if _, ok := res.Labels["vpc_id"]; ok {
		t.Fatal("expected no vpc_id when Metadata empty")
	}
	if _, ok := res.Labels["az"]; ok {
		t.Fatal("expected no az when OSEXTAZavailabilityZone empty")
	}
}

func TestMapRDSInstanceEnrichmentLabels(t *testing.T) {
	res := mapRDSInstance("cn-south-1", rdsmodel.InstanceResponse{
		Id:         "rds-2",
		Name:       "db-replica",
		Status:     "ACTIVE",
		PrivateIps: []string{"192.168.1.10", "192.168.1.11"},
		VpcId:      "vpc-rds",
		SubnetId:   "subnet-rds",
		FlavorRef:  "rds.pg.c2.medium",
	})
	if res.Labels["private_ip"] != "192.168.1.10" {
		t.Fatalf("private_ip = %q, want 192.168.1.10", res.Labels["private_ip"])
	}
	if res.Labels["vpc_id"] != "vpc-rds" {
		t.Fatalf("vpc_id = %q, want vpc-rds", res.Labels["vpc_id"])
	}
	if res.Labels["subnet_id"] != "subnet-rds" {
		t.Fatalf("subnet_id = %q, want subnet-rds", res.Labels["subnet_id"])
	}
	if res.Labels["flavor"] != "rds.pg.c2.medium" {
		t.Fatalf("flavor = %q, want rds.pg.c2.medium", res.Labels["flavor"])
	}
}

func TestMapEVSVolume(t *testing.T) {
	// 含挂载与包周期计费。
	res := mapEVSVolume("cn-south-1", evsmodel.VolumeDetail{
		Id:               "vol-1",
		Name:             "disk-data-01",
		Status:           "in-use",
		VolumeType:       "SSD",
		Size:             100,
		AvailabilityZone: "cn-south-1a",
		CreatedAt:        "2024-01-01T00:00:00.000000",
		Attachments:      []evsmodel.Attachment{{ServerId: "srv-abc"}},
		Metadata:         map[string]interface{}{"orderID": "ORDER123"},
	})
	if res.Type != "evs" || res.ProviderRef != "disk-data-01" {
		t.Fatalf("evs base: %+v", res)
	}
	if res.Labels["volume_id"] != "vol-1" {
		t.Fatalf("volume_id = %q", res.Labels["volume_id"])
	}
	if res.Labels["volume_type"] != "SSD" {
		t.Fatalf("volume_type = %q", res.Labels["volume_type"])
	}
	if res.Labels["size_gb"] != "100" {
		t.Fatalf("size_gb = %q", res.Labels["size_gb"])
	}
	if res.Labels["attached_to"] != "srv-abc" {
		t.Fatalf("attached_to = %q", res.Labels["attached_to"])
	}
	if res.Labels["az"] != "cn-south-1a" {
		t.Fatalf("az = %q", res.Labels["az"])
	}
	if res.Labels["charging_mode"] != "prepaid" {
		t.Fatalf("charging_mode = %q, want prepaid", res.Labels["charging_mode"])
	}
	// 无 orderID → 按需。
	res2 := mapEVSVolume("cn-south-1", evsmodel.VolumeDetail{Id: "vol-2", Name: "d2", Size: 50, VolumeType: "SATA"})
	if res2.Labels["charging_mode"] != "postpaid" {
		t.Fatalf("charging_mode = %q, want postpaid", res2.Labels["charging_mode"])
	}
}

func TestMapVPC(t *testing.T) {
	res := mapVPC("cn-north-4", vpcmodel.Vpc{
		Id:                  "vpc-1",
		Name:                "prod-vpc",
		Cidr:                "192.168.0.0/16",
		Status:              vpcmodel.GetVpcStatusEnum().OK,
		EnterpriseProjectId: "eps-xxx",
	}, 3)
	if res.Type != "vpc" || res.ProviderRef != "vpc-1" {
		t.Fatalf("vpc base: %+v", res)
	}
	if res.Labels["vpc_name"] != "prod-vpc" {
		t.Fatalf("vpc_name = %q", res.Labels["vpc_name"])
	}
	if res.Labels["cidr"] != "192.168.0.0/16" {
		t.Fatalf("cidr = %q", res.Labels["cidr"])
	}
	if res.Labels["status"] != "OK" {
		t.Fatalf("status = %q", res.Labels["status"])
	}
	if res.Labels["enterprise_project_id"] != "eps-xxx" {
		t.Fatalf("enterprise_project_id = %q", res.Labels["enterprise_project_id"])
	}
	// subnet_count 由 listVPC 回填，mapVPC 不直接写入（传 0 时不应出现）。
	if _, ok := res.Labels["subnet_count"]; ok {
		t.Fatalf("subnet_count should not be set by mapVPC, got %q", res.Labels["subnet_count"])
	}
}

func TestMapEIP(t *testing.T) {
	active := eipmodel.GetPublicipShowRespStatusEnum().ACTIVE
	shareType := eipmodel.GetPublicipShowRespBandwidthShareTypeEnum().PER
	size := int32(5)
	res := mapEIP("cn-south-1", eipmodel.PublicipShowResp{
		Id:                  strPtr("eip-1"),
		Alias:               strPtr("prod-eip"),
		PublicIpAddress:     strPtr("1.2.3.4"),
		PrivateIpAddress:    strPtr("10.0.0.4"),
		BandwidthId:         strPtr("bw-1"),
		BandwidthShareType:  &shareType,
		BandwidthSize:       &size,
		Type:                strPtr("5_bgp"),
		EnterpriseProjectId: strPtr("eps-1"),
		Status:              &active,
	})
	if res.Type != "eip" || res.ProviderRef != "eip-1" {
		t.Fatalf("eip base: %+v", res)
	}
	if res.Name != "prod-eip" {
		t.Fatalf("Name = %q, want prod-eip", res.Name)
	}
	if res.Status != "ACTIVE" {
		t.Fatalf("Status = %q, want ACTIVE", res.Status)
	}
	if res.Labels["public_ip"] != "1.2.3.4" {
		t.Fatalf("public_ip = %q", res.Labels["public_ip"])
	}
	if res.Labels["private_ip"] != "10.0.0.4" {
		t.Fatalf("private_ip = %q", res.Labels["private_ip"])
	}
	if res.Labels["bandwidth_id"] != "bw-1" {
		t.Fatalf("bandwidth_id = %q", res.Labels["bandwidth_id"])
	}
	if res.Labels["share_type"] != "PER" {
		t.Fatalf("share_type = %q", res.Labels["share_type"])
	}
	// Name 兜底到 public_ip。
	res2 := mapEIP("r", eipmodel.PublicipShowResp{Id: strPtr("eip-2"), PublicIpAddress: strPtr("9.9.9.9")})
	if res2.Name != "9.9.9.9" {
		t.Fatalf("Name fallback = %q, want 9.9.9.9", res2.Name)
	}
}

func TestMapBandwidth(t *testing.T) {
	normal := eipmodel.GetBandwidthRespStatusEnum().NORMAL
	shareType := eipmodel.GetBandwidthRespShareTypeEnum().WHOLE
	chargeMode := eipmodel.GetBandwidthRespChargeModeEnum().TRAFFIC
	size := int32(100)
	res := mapBandwidth("cn-north-4", eipmodel.BandwidthResp{
		Id:                  strPtr("bw-1"),
		Name:                strPtr("shared-bw"),
		Size:                &size,
		ShareType:           &shareType,
		ChargeMode:          &chargeMode,
		Status:              &normal,
		EnterpriseProjectId: strPtr("eps-2"),
	})
	if res.Type != "bandwidth" || res.ProviderRef != "bw-1" {
		t.Fatalf("bandwidth base: %+v", res)
	}
	if res.Name != "shared-bw" {
		t.Fatalf("Name = %q, want shared-bw", res.Name)
	}
	if res.Status != "NORMAL" {
		t.Fatalf("Status = %q, want NORMAL", res.Status)
	}
	if res.Labels["size_mbps"] != "100" {
		t.Fatalf("size_mbps = %q", res.Labels["size_mbps"])
	}
	if res.Labels["share_type"] != "WHOLE" {
		t.Fatalf("share_type = %q", res.Labels["share_type"])
	}
	if res.Labels["charge_mode"] != "traffic" {
		t.Fatalf("charge_mode = %q", res.Labels["charge_mode"])
	}
}

func TestMapSubnet(t *testing.T) {
	res := mapSubnet("cn-south-1", vpcmodel.Subnet{
		Id:                      "sub-1",
		Name:                    "web-subnet",
		Cidr:                    "192.168.1.0/24",
		GatewayIp:               "192.168.1.1",
		VpcId:                   "vpc-1",
		AvailabilityZone:        "cn-south-1a",
		AvailableIpAddressCount: 250,
		Status:                  vpcmodel.GetSubnetStatusEnum().ACTIVE,
	})
	if res.Type != "subnet" || res.ProviderRef != "sub-1" {
		t.Fatalf("subnet base: %+v", res)
	}
	if res.Name != "web-subnet" {
		t.Fatalf("Name = %q, want web-subnet", res.Name)
	}
	if res.Labels["cidr"] != "192.168.1.0/24" {
		t.Fatalf("cidr = %q", res.Labels["cidr"])
	}
	if res.Labels["gateway_ip"] != "192.168.1.1" {
		t.Fatalf("gateway_ip = %q", res.Labels["gateway_ip"])
	}
	if res.Labels["vpc_id"] != "vpc-1" {
		t.Fatalf("vpc_id = %q", res.Labels["vpc_id"])
	}
	if res.Labels["az"] != "cn-south-1a" {
		t.Fatalf("az = %q", res.Labels["az"])
	}
	if res.Labels["available_ip_count"] != "250" {
		t.Fatalf("available_ip_count = %q", res.Labels["available_ip_count"])
	}
}

func TestMapPeering(t *testing.T) {
	res := mapPeering("cn-north-4", vpcmodel.VpcPeering{
		Id:             "peer-1",
		Name:           "vpc-a-to-vpc-b",
		Status:         vpcmodel.GetVpcPeeringStatusEnum().ACTIVE,
		RequestVpcInfo: &vpcmodel.VpcInfo{VpcId: "vpc-a"},
		AcceptVpcInfo:  &vpcmodel.VpcInfo{VpcId: "vpc-b"},
	})
	if res.Type != "peering" || res.ProviderRef != "peer-1" {
		t.Fatalf("peering base: %+v", res)
	}
	if res.Name != "vpc-a-to-vpc-b" {
		t.Fatalf("Name = %q, want vpc-a-to-vpc-b", res.Name)
	}
	if res.Status != "ACTIVE" {
		t.Fatalf("Status = %q, want ACTIVE", res.Status)
	}
	if res.Labels["request_vpc_id"] != "vpc-a" {
		t.Fatalf("request_vpc_id = %q", res.Labels["request_vpc_id"])
	}
	if res.Labels["accept_vpc_id"] != "vpc-b" {
		t.Fatalf("accept_vpc_id = %q", res.Labels["accept_vpc_id"])
	}
}

func TestMapDCSInstance(t *testing.T) {
	engine := "Redis"
	ver := "5.0"
	cap := int32(2)
	spec := "dcs.master_standby"
	ip := "192.168.0.10"
	vpc := "vpc-dcs"
	cm := int32(1) // 包周期
	created := "2024-02-01T00:00:00"
	status := "RUNNING"
	azs := []string{"cn-south-1a", "cn-south-1b"}
	res := mapDCSInstance("cn-south-1", dcsmodel.InstanceListInfo{
		InstanceId:    strPtr("dcs-1"),
		Name:          strPtr("cache-prod"),
		Engine:        &engine,
		EngineVersion: &ver,
		Capacity:      &cap,
		SpecCode:      &spec,
		Ip:            &ip,
		VpcId:         &vpc,
		ChargingMode:  &cm,
		CreatedAt:     &created,
		Status:        &status,
		AzCodes:       &azs,
	})
	if res.Type != "dcs" || res.ProviderRef != "dcs-1" {
		t.Fatalf("dcs base: %+v", res)
	}
	if res.Labels["engine"] != "Redis" {
		t.Fatalf("engine = %q", res.Labels["engine"])
	}
	if res.Labels["capacity_gb"] != "2" {
		t.Fatalf("capacity_gb = %q", res.Labels["capacity_gb"])
	}
	if res.Labels["charging_mode"] != "prepaid" {
		t.Fatalf("charging_mode = %q, want prepaid (DCS 1=包周期)", res.Labels["charging_mode"])
	}
	if res.Labels["az"] != "cn-south-1a,cn-south-1b" {
		t.Fatalf("az = %q", res.Labels["az"])
	}
}

func TestMapDMSKafkaInstance(t *testing.T) {
	cm := int32(0) // Kafka 0=包周期
	space := int32(300)
	res := mapDMSKafkaInstance("cn-north-4", kafkamodel.ShowInstanceResp{
		InstanceId:       strPtr("kafka-1"),
		Name:             strPtr("mq-prod"),
		Engine:           strPtr("kafka"),
		EngineVersion:    strPtr("2.7"),
		ResourceSpecCode: strPtr("dms.instance.kafka.cluster.c3.high.2"),
		StorageSpace:     &space,
		ConnectAddress:   strPtr("192.168.1.10:9092"),
		VpcId:            strPtr("vpc-kafka"),
		ChargingMode:     &cm,
		Status:           strPtr("RUNNING"),
	})
	if res.Type != "dms" || res.ProviderRef != "kafka-1" {
		t.Fatalf("dms kafka base: %+v", res)
	}
	if res.Labels["spec_code"] != "dms.instance.kafka.cluster.c3.high.2" {
		t.Fatalf("spec_code = %q", res.Labels["spec_code"])
	}
	if res.Labels["capacity_gb"] != "300" {
		t.Fatalf("capacity_gb = %q", res.Labels["capacity_gb"])
	}
	if res.Labels["charging_mode"] != "prepaid" {
		t.Fatalf("charging_mode = %q, want prepaid (Kafka 0=包周期)", res.Labels["charging_mode"])
	}
	// Kafka 无 az 字段。
	if _, ok := res.Labels["az"]; ok {
		t.Fatalf("kafka should not have az label")
	}
}

func TestMapDMSRocketMQInstance(t *testing.T) {
	cm := int32(1) // RocketMQ 1=按需
	space := int32(200)
	azs := []string{"cn-north-4a"}
	res := mapDMSRocketMQInstance("cn-north-4", rocketmqmodel.InstanceDetail{
		InstanceId:         strPtr("rmq-1"),
		Name:               strPtr("rocket-prod"),
		Engine:             strPtr("rocketmq"),
		EngineVersion:      strPtr("4.8.0"),
		Specification:      strPtr("c6.4u8g.cluster"),
		StorageSpace:       &space,
		VpcId:              strPtr("vpc-rmq"),
		ChargingMode:       &cm,
		Status:             strPtr("RUNNING"),
		AvailableZoneNames: &azs,
	})
	if res.Type != "dms" || res.ProviderRef != "rmq-1" {
		t.Fatalf("dms rocketmq base: %+v", res)
	}
	if res.Labels["charging_mode"] != "postpaid" {
		t.Fatalf("charging_mode = %q, want postpaid (RocketMQ 1=按需)", res.Labels["charging_mode"])
	}
	if res.Labels["az"] != "cn-north-4a" {
		t.Fatalf("az = %q", res.Labels["az"])
	}
	// RocketMQ 无 connect_address。
	if _, ok := res.Labels["private_ip"]; ok {
		t.Fatalf("rocketmq should not have private_ip label")
	}
}
