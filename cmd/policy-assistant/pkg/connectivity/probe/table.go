package probe

import (
	"fmt"
	"strings"

	"github.com/mattfenwick/collections/pkg/slice"
	"github.com/pkg/errors"
	"golang.org/x/exp/maps"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/network-policy-api/policy-assistant/pkg/utils"
)

type Item struct {
	From       string
	To         string
	JobResults map[string]*JobResult
}

func (p *Item) AddJobResult(jr *JobResult) error {
	if _, ok := p.JobResults[jr.Key()]; ok {
		return errors.Errorf("unable to add job result: duplicate key %s (job %+v)", jr.Key(), jr.Job)
	}
	p.JobResults[jr.Key()] = jr
	return nil
}

type Table struct {
	Wrapped *TruthTable
}

func NewTableWithDefaultConnectivity(r *Resources, ingress, egress Connectivity) *Table {
	return &Table{Wrapped: NewTruthTableFromItems(r.SortedPodNames(), func(fr, to string) interface{} {
		results := make(map[string]*JobResult, len(r.ports)*len(r.protocols))
		for _, proto := range r.protocols {
			for _, port := range r.ports {
				jr := &JobResult{
					Job: &Job{
						FromKey:          fr,
						ToKey:            to,
						ResolvedPort:     port,
						ResolvedPortName: "",
						Protocol:         proto,
						TimeoutSeconds:   3,
					},
					Ingress: &ingress,
					Egress:  &egress,
				}

				if fr == to {
					c := ConnectivityUndefined
					jr.Ingress = &c
					jr.Egress = &c
				}

				setCombined(jr)

				k := fmt.Sprintf("%s/%d", proto, port)
				results[k] = jr
			}
		}

		return &Item{
			From:       fr,
			To:         to,
			JobResults: results,
		}
	})}
}

// SetEgress should be used to set connectivity for tables with connectivity already specified via NewTableWithDefaultConnectivity()
func (t *Table) SetEgress(egress Connectivity, from, to string, port int, proto v1.Protocol) {
	portProto := fmt.Sprintf("%s/%d", proto, port)
	jr, ok := t.Get(from, to).JobResults[portProto]
	if !ok || jr.Ingress == nil || jr.Egress == nil {
		panic(errors.Errorf("cannot set connectivity: job result non-existent/invalid for %s/%d", proto, port))
	}

	jr.Egress = &egress
	setCombined(jr)
}

// SetIngress should be used to set connectivity for tables with connectivity already specified via NewTableWithDefaultConnectivity()
func (t *Table) SetIngress(ingress Connectivity, from, to string, port int, proto v1.Protocol) {
	portProto := fmt.Sprintf("%s/%d", proto, port)
	jr, ok := t.Get(from, to).JobResults[portProto]
	if !ok || jr.Ingress == nil || jr.Egress == nil {
		panic(errors.Errorf("cannot set connectivity: job result non-existent/invalid for %s/%d", proto, port))
	}

	jr.Ingress = &ingress
	setCombined(jr)
}

func setCombined(jr *JobResult) {
	if *jr.Ingress == ConnectivityBlocked || *jr.Egress == ConnectivityBlocked {
		jr.Combined = ConnectivityBlocked
	}

	if *jr.Ingress == ConnectivityAllowed && *jr.Egress == ConnectivityAllowed {
		jr.Combined = ConnectivityAllowed
	}
}

func NewTable(items []string) *Table {
	return &Table{Wrapped: NewTruthTableFromItems(items, func(fr, to string) interface{} {
		return &Item{
			From:       fr,
			To:         to,
			JobResults: map[string]*JobResult{},
		}
	})}
}

func NewTableFromJobResults(resources *Resources, jobResults []*JobResult) *Table {
	table := NewTable(resources.SortedPodNames())
	for _, result := range jobResults {
		fr := result.Job.FromKey
		to := result.Job.ToKey
		pp := table.Get(fr, to)
		// this really shouldn't happen, so let's not recover from it
		utils.DoOrDie(pp.AddJobResult(result))
	}
	return table
}

//func (t *Table) Set(from string, to string, value *Item) {
//	t.Wrapped.Set(from, to, value)
//}

func (t *Table) Get(from string, to string) *Item {
	return t.Wrapped.Get(from, to).(*Item)
}

func (t *Table) RenderIngress() string {
	return t.renderTableHelper(getIngress)
}

func (t *Table) RenderEgress() string {
	return t.renderTableHelper(getEgress)
}

func (t *Table) RenderTable() string {
	return t.renderTableHelper(getCombined)
}

func (t *Table) renderTableHelper(render func(*JobResult) string) string {
	isSchemaUniform, isSingleElement := true, true
	schema := map[string]bool{}

	for _, key := range t.Wrapped.Keys() {
		dict := t.Get(key.From, key.To).JobResults
		if len(dict) != 1 {
			isSingleElement = false
			break
		}
		keys := slice.Sort(maps.Keys(dict))
		schema[strings.Join(keys, "_")] = true
		if len(schema) > 1 {
			isSchemaUniform = false
			break
		}
	}
	if isSchemaUniform && isSingleElement {
		return t.renderSimpleTable(render)
	} else if isSchemaUniform {
		return t.renderUniformMultiTable(render)
	} else {
		return t.renderNonuniformTable(render)
	}
}

func getCombined(result *JobResult) string {
	return result.Combined.ShortString()
}

func getIngress(result *JobResult) string {
	return result.Ingress.ShortString()
}

func getEgress(result *JobResult) string {
	return result.Egress.ShortString()
}

func (t *Table) renderSimpleTable(render func(*JobResult) string) string {
	return t.Wrapped.Table("", false, func(fr, to string, i interface{}) string {
		var v *JobResult
		for _, value := range t.Get(fr, to).JobResults {
			v = value
			break
		}
		return render(v)
	})
}

func (t *Table) renderUniformMultiTable(render func(*JobResult) string) string {
	key := t.Wrapped.Keys()[0]
	first := t.Get(key.From, key.To)
	keys := slice.Sort(maps.Keys(first.JobResults))
	schema := strings.Join(keys, "\n")
	return t.Wrapped.Table(schema, true, func(fr, to string, i interface{}) string {
		dict := t.Get(fr, to).JobResults
		var lines []string
		for _, k := range keys {
			lines = append(lines, render(dict[k]))
		}
		return strings.Join(lines, " ")
	})
}

func (t *Table) renderNonuniformTable(render func(*JobResult) string) string {
	return t.Wrapped.Table("", true, func(fr, to string, i interface{}) string {
		dict := t.Get(fr, to).JobResults
		keys := slice.Sort(maps.Keys(dict))
		var lines []string
		for _, k := range keys {
			lines = append(lines, k+": "+render(dict[k]))
		}
		return strings.Join(lines, "\n")
	})
}
