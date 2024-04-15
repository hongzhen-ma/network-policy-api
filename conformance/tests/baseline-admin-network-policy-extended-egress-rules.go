/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/network-policy-api/apis/v1alpha1"
	"sigs.k8s.io/network-policy-api/conformance/utils/kubernetes"
	"sigs.k8s.io/network-policy-api/conformance/utils/suite"
)

func init() {
	ConformanceTests = append(ConformanceTests,
		BaselineAdminNetworkPolicyEgressNamedPort,
		BaselineAdminNetworkPolicyEgressNodePeers,
		BaselineAdminNetworkPolicyEgressInlineCIDRPeers,
	)
}

var BaselineAdminNetworkPolicyEgressNamedPort = suite.ConformanceTest{
	ShortName:   "BaselineAdminNetworkPolicyEgressNamedPort",
	Description: "Tests support for egress traffic on a named port using baseline admin network policy API based on a server and client model",
	Features: []suite.SupportedFeature{
		suite.SupportBaselineAdminNetworkPolicy,
		suite.SupportBaselineAdminNetworkPolicyNamedPorts,
	},
	Manifests: []string{"base/baseline_admin_network_policy/core-egress-udp-rules.yaml"},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {

		t.Run("Should support an 'allow-egress' policy for named port", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), s.TimeoutConfig.GetTimeout)
			defer cancel()
			// This test uses `default` BANP
			// harry-potter-1 is our server pod in gryffindor namespace
			serverPod := &v1.Pod{}
			err := s.Client.Get(ctx, client.ObjectKey{
				Namespace: "network-policy-conformance-gryffindor",
				Name:      "harry-potter-1",
			}, serverPod)
			require.NoErrorf(t, err, "unable to fetch the server pod")
			banp := &v1alpha1.BaselineAdminNetworkPolicy{}
			err = s.Client.Get(ctx, client.ObjectKey{
				Name: "default",
			}, banp)
			require.NoErrorf(t, err, "unable to fetch the baseline admin network policy")
			mutate := banp.DeepCopy()
			dnsPortRule := mutate.Spec.Egress[3]
			dnsPort := "dns"
			// rewrite the udp port 53 rule as named port rule
			dnsPortRule.Ports = &[]v1alpha1.AdminNetworkPolicyPort{
				{
					NamedPort: &dnsPort,
				},
			}
			mutate.Spec.Egress[3] = dnsPortRule
			err = s.Client.Patch(ctx, mutate, client.MergeFrom(banp))
			require.NoErrorf(t, err, "unable to patch the baseline admin network policy")
			// cedric-diggory-0 is our client pod in hufflepuff namespace
			// ensure egress is ALLOWED to gryffindor from hufflepuff at the dns port, which is defined as UDP at port 53 in pod spec
			// modified ingressRule at index3 should take effect
			success := kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-hufflepuff", "cedric-diggory-0", "udp",
				serverPod.Status.PodIP, int32(53), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			// cedric-diggory-1 is our client pod in hufflepuff namespace
			// ensure egress is DENIED to gryffindor from hufflepuff for rest of the traffic; egressRule at index4 should take effect
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-hufflepuff", "cedric-diggory-1", "udp",
				serverPod.Status.PodIP, int32(5353), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
		})

	},
}

var BaselineAdminNetworkPolicyEgressNodePeers = suite.ConformanceTest{
	ShortName:   "BaselineAdminNetworkPolicyEgressNodePeers",
	Description: "Tests support for egress traffic to node peers using  baseline admin network policy API based on a server and client model",
	Features: []suite.SupportedFeature{
		suite.SupportBaselineAdminNetworkPolicy,
		suite.SupportBaselineAdminNetworkPolicyEgressNodePeers,
	},
	Manifests: []string{"base/baseline_admin_network_policy/extended-egress-selector-rules.yaml"},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {
		ctx, cancel := context.WithTimeout(context.Background(), s.TimeoutConfig.GetTimeout)
		defer cancel()
		// centaur-1 is our server host-networked pod in forbidden-forrest namespace
		serverPod := &v1.Pod{}
		err := s.Client.Get(ctx, client.ObjectKey{
			Namespace: "network-policy-conformance-forbidden-forrest",
			Name:      "centaur-1",
		}, serverPod)
		require.NoErrorf(t, err, "unable to fetch the server pod")
		t.Run("Should support an 'allow-egress' rule policy for egress-node-peer", func(t *testing.T) {
			// harry-potter-0 is our client pod in gryffindor namespace
			// ensure egress is ALLOWED to forbidden-forrest from gryffindor at the 36363 TCP port
			// egressRule at index0 should take effect
			success := kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-0", "tcp",
				serverPod.Status.PodIP, int32(36363), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "tcp",
				serverPod.Status.PodIP, int32(36364), s.TimeoutConfig.RequestTimeout, true) // Pass rule at index2 takes effect
			assert.True(t, success)
		})
		t.Run("Should support a 'deny-egress' rule policy for egress-node-peer", func(t *testing.T) {
			// harry-potter-1 is our client pod in gryffindor namespace
			// ensure egress is DENIED to rest of the nodes from gryffindor; egressRule at index1 should take effect
			success := kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "udp",
				serverPod.Status.PodIP, int32(34346), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "sctp",
				serverPod.Status.PodIP, int32(9003), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
		})
	},
}

var BaselineAdminNetworkPolicyEgressInlineCIDRPeers = suite.ConformanceTest{
	ShortName:   "BaselineAdminNetworkPolicyEgressInlineCIDRPeers",
	Description: "Tests support for egress traffic to CIDR peers using baseline admin network policy API based on a server and client model",
	Features: []suite.SupportedFeature{
		suite.SupportBaselineAdminNetworkPolicy,
		suite.SupportBaselineAdminNetworkPolicyEgressInlineCIDRPeers,
	},
	Manifests: []string{"base/baseline_admin_network_policy/extended-egress-selector-rules.yaml"},
	Test: func(t *testing.T, s *suite.ConformanceTestSuite) {
		ctx, cancel := context.WithTimeout(context.Background(), s.TimeoutConfig.GetTimeout)
		defer cancel()
		t.Run("Should support a 'deny-egress' rule policy for egress-cidr-peer", func(t *testing.T) {
			// harry-potter-1 is our client pod in gryffindor namespace
			// Let us pick a pod in ravenclaw namespace and try to connect, it won't work
			// ensure egress is DENIED to 0.0.0.0/0 from gryffindor; egressRule at index1 should take effect
			// luna-lovegood-0 is our server pod in ravenclaw namespace
			serverPod := &v1.Pod{}
			err := s.Client.Get(ctx, client.ObjectKey{
				Namespace: "network-policy-conformance-ravenclaw",
				Name:      "luna-lovegood-0",
			}, serverPod)
			require.NoErrorf(t, err, "unable to fetch the server pod")
			success := kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "tcp",
				serverPod.Status.PodIP, int32(80), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "udp",
				serverPod.Status.PodIP, int32(53), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "sctp",
				serverPod.Status.PodIP, int32(9003), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			// Let us pick a pod in hufflepuff namespace and try to connect, it won't work
			// ensure egress is DENIED to 0.0.0.0/0 from gryffindor; egressRule at index2 should take effect
			// cedric-diggory-0 is our server pod in hufflepuff namespace
			serverPod = &v1.Pod{}
			err = s.Client.Get(ctx, client.ObjectKey{
				Namespace: "network-policy-conformance-hufflepuff",
				Name:      "cedric-diggory-0",
			}, serverPod)
			require.NoErrorf(t, err, "unable to fetch the server pod")
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "tcp",
				serverPod.Status.PodIP, int32(80), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "udp",
				serverPod.Status.PodIP, int32(53), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "sctp",
				serverPod.Status.PodIP, int32(9003), s.TimeoutConfig.RequestTimeout, false)
			assert.True(t, success)
		})
		// To test allow CIDR rule, insert the following rule at index0
		//- name: "allow-egress-to-specific-podIPs"
		//  action: "Allow"
		//  to:
		//  - networks:
		//	  - luna-lovegood-0.IP
		//    - cedric-diggory-0.IP
		t.Run("Should support an 'allow-egress' rule policy for egress-cidr-peer", func(t *testing.T) {
			serverPodRavenclaw := &v1.Pod{}
			err := s.Client.Get(ctx, client.ObjectKey{
				Namespace: "network-policy-conformance-ravenclaw",
				Name:      "luna-lovegood-0",
			}, serverPodRavenclaw)
			require.NoErrorf(t, err, "unable to fetch the server pod")
			serverPodHufflepuff := &v1.Pod{}
			err = s.Client.Get(ctx, client.ObjectKey{
				Namespace: "network-policy-conformance-hufflepuff",
				Name:      "cedric-diggory-0",
			}, serverPodHufflepuff)
			require.NoErrorf(t, err, "unable to fetch the server pod")
			banp := &v1alpha1.BaselineAdminNetworkPolicy{}
			err = s.Client.Get(ctx, client.ObjectKey{
				Name: "default",
			}, banp)
			require.NoErrorf(t, err, "unable to fetch the baseline admin network policy")
			mutate := banp.DeepCopy()
			var mask string
			if net.IsIPv4String(serverPodRavenclaw.Status.PodIP) {
				mask = "/32"
			} else {
				mask = "/128"
			}
			// insert new rule at index0; append the rest of the rules in the default BANP
			newRule := []v1alpha1.BaselineAdminNetworkPolicyEgressRule{
				{
					Name:   "allow-egress-to-specific-podIPs",
					Action: "Allow",
					To: []v1alpha1.AdminNetworkPolicyEgressPeer{
						{
							Networks: []v1alpha1.CIDR{
								v1alpha1.CIDR(serverPodRavenclaw.Status.PodIP + mask),
								v1alpha1.CIDR(serverPodHufflepuff.Status.PodIP + mask),
							},
						},
					},
				},
			}
			mutate.Spec.Egress = append(newRule, mutate.Spec.Egress...)
			err = s.Client.Patch(ctx, mutate, client.MergeFrom(banp))
			require.NoErrorf(t, err, "unable to patch the baseline admin network policy")
			// harry-potter-0 is our client pod in gryffindor namespace
			// ensure egress is ALLOWED to luna-lovegood-0.IP and cedric-diggory-0.IP
			// new egressRule at index0 should take effect
			success := kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "tcp",
				serverPodRavenclaw.Status.PodIP, int32(80), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "udp",
				serverPodRavenclaw.Status.PodIP, int32(53), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "sctp",
				serverPodRavenclaw.Status.PodIP, int32(9003), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "tcp",
				serverPodHufflepuff.Status.PodIP, int32(80), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "udp",
				serverPodHufflepuff.Status.PodIP, int32(53), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
			success = kubernetes.PokeServer(t, s.ClientSet, &s.KubeConfig, "network-policy-conformance-gryffindor", "harry-potter-1", "sctp",
				serverPodHufflepuff.Status.PodIP, int32(9003), s.TimeoutConfig.RequestTimeout, true)
			assert.True(t, success)
		})
	},
}
