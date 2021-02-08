package cli

import (
	"github.com/mattfenwick/cyclonus/pkg/connectivity"
	"github.com/mattfenwick/cyclonus/pkg/connectivity/types"
	"github.com/mattfenwick/cyclonus/pkg/generator"
	"github.com/mattfenwick/cyclonus/pkg/kube"
	"github.com/mattfenwick/cyclonus/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"
)

type ProbeArgs struct {
	Noisy                     bool
	IgnoreLoopback            bool
	KubeContext               string
	PerturbationWaitSeconds   int
	PodCreationTimeoutSeconds int
	PolicyPath                string

	// what to probe on
	ProbeAllAvailable bool
	Ports             []string
	Protocols         []string

	// server setup
	ServerProtocols  []string
	ServerPorts      []int
	ServerNamespaces []string
	ServerPods       []string
}

func SetupProbeCommand() *cobra.Command {
	args := &ProbeArgs{}

	command := &cobra.Command{
		Use:   "probe",
		Short: "run a connectivity probe against kubernetes pods",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, as []string) {
			RunProbeCommand(args)
		},
	}

	command.Flags().StringSliceVarP(&args.ServerNamespaces, "server-namespace", "n", []string{"x", "y", "z"}, "namespaces to create/use pods in")
	command.Flags().StringSliceVar(&args.ServerPods, "server-pod", []string{"a", "b", "c"}, "pods to create in namespaces")
	command.Flags().IntSliceVar(&args.ServerPorts, "server-port", []int{80, 81}, "ports to run server on")
	// TODO add UDP to defaults once support has been added
	command.Flags().StringSliceVar(&args.ServerProtocols, "server-protocol", []string{"tcp"}, "protocols to run server on")

	command.Flags().BoolVar(&args.ProbeAllAvailable, "all-available", false, "if true, probe all available ports and protocols on each pod")
	command.Flags().StringSliceVar(&args.Ports, "port", []string{"80"}, "ports to run probes on; may be named port or numbered port")
	command.Flags().StringSliceVar(&args.Protocols, "protocol", []string{"tcp"}, "protocols to run probes on")

	command.Flags().BoolVar(&args.Noisy, "noisy", false, "if true, print all results")
	command.Flags().BoolVar(&args.IgnoreLoopback, "ignore-loopback", false, "if true, ignore loopback for truthtable correctness verification")
	command.Flags().StringVar(&args.KubeContext, "context", "", "kubernetes context to use; if empty, uses default context")
	command.Flags().IntVar(&args.PerturbationWaitSeconds, "perturbation-wait-seconds", 5, "number of seconds to wait after perturbing the cluster (i.e. create a network policy, modify a ns/pod label) before running probes, to give the CNI time to update the cluster state")
	command.Flags().IntVar(&args.PodCreationTimeoutSeconds, "pod-creation-timeout-seconds", 60, "number of seconds to wait for pods to create, be running and have IP addresses")
	command.Flags().StringVar(&args.PolicyPath, "policy-path", "", "path to yaml network policy to create in kube; if empty, will not create any policies")

	return command
}

func RunProbeCommand(args *ProbeArgs) {
	externalIPs := []string{"http://www.google.com"} // TODO make these be IPs?  or not?
	if len(args.ServerNamespaces) == 0 || len(args.ServerPods) == 0 {
		panic(errors.Errorf("found 0 namespaces or pods, must have at least 1 of each"))
	}

	kubernetes, err := kube.NewKubernetesForContext(args.KubeContext)
	utils.DoOrDie(err)

	protocols := parseProtocols(args.Protocols)
	serverProtocols := parseProtocols(args.ServerProtocols)

	resources, err := types.NewDefaultResources(kubernetes, args.ServerNamespaces, args.ServerPods, args.ServerPorts, serverProtocols, externalIPs, args.PodCreationTimeoutSeconds)
	utils.DoOrDie(err)
	interpreter, err := connectivity.NewInterpreter(kubernetes, resources, false, 0, args.PerturbationWaitSeconds, false)
	utils.DoOrDie(err)

	actions := []*generator.Action{generator.ReadNetworkPolicies(args.ServerNamespaces)}

	if args.PolicyPath != "" {
		policyBytes, err := ioutil.ReadFile(args.PolicyPath)
		utils.DoOrDie(err)

		var kubePolicy networkingv1.NetworkPolicy
		err = yaml.Unmarshal(policyBytes, &kubePolicy)
		utils.DoOrDie(err)

		actions = append(actions, generator.CreatePolicy(&kubePolicy))
	}

	printer := connectivity.Printer{
		Noisy:          args.Noisy,
		IgnoreLoopback: args.IgnoreLoopback,
	}

	if args.ProbeAllAvailable {
		result := interpreter.ExecuteTestCase(generator.NewSingleStepTestCase("all available one-off probe", &generator.ProbeConfig{AllAvailable: true}, actions...))
		printer.PrintTestCaseResult(result)
	} else {
		for _, port := range args.Ports {
			for _, protocol := range protocols {
				parsedPort := intstr.Parse(port)
				pp := &generator.PortProtocol{
					Protocol: protocol,
					Port:     parsedPort,
				}
				probeConfig := &generator.ProbeConfig{PortProtocol: pp}
				result := interpreter.ExecuteTestCase(generator.NewSingleStepTestCase("specific port/protocol one-off probe", probeConfig, actions...))

				printer.PrintTestCaseResult(result)
			}
		}
	}
}

func parseProtocols(strs []string) []v1.Protocol {
	var protocols []v1.Protocol
	for _, protocol := range strs {
		parsedProtocol, err := kube.ParseProtocol(protocol)
		utils.DoOrDie(err)
		protocols = append(protocols, parsedProtocol)
	}
	return protocols
}
