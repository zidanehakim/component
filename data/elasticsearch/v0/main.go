//go:generate compogen readme ./config ./README.mdx
package elasticsearch

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/instill-ai/component/base"
	"github.com/instill-ai/x/errmsg"
)

const (
	TaskSearch       = "TASK_SEARCH"
	TaskVectorSearch = "TASK_VECTOR_SEARCH"
	TaskIndex        = "TASK_INDEX"
	TaskUpdate       = "TASK_UPDATE"
	TaskDelete       = "TASK_DELETE"
	TaskCreateIndex  = "TASK_CREATE_INDEX"
	TaskDeleteIndex  = "TASK_DELETE_INDEX"
)

var (
	//go:embed config/definition.json
	definitionJSON []byte
	//go:embed config/setup.json
	setupJSON []byte
	//go:embed config/tasks.json
	tasksJSON []byte

	once sync.Once
	comp *component
)

type component struct {
	base.Component
}

type execution struct {
	base.ComponentExecution

	execute func(*structpb.Struct) (*structpb.Struct, error)
	client  ESClient
}

type ESClient struct {
	indexClient       esapi.Index
	searchClient      esapi.Search
	updateClient      esapi.UpdateByQuery
	deleteClient      esapi.DeleteByQuery
	createIndexClient esapi.IndicesCreate
	deleteIndexClient esapi.IndicesDelete
	countClient       esapi.Count
}

type ESSearch func(o ...func(*esapi.SearchRequest)) (*esapi.Response, error)

type ESIndex func(index string, body io.Reader, o ...func(*esapi.IndexRequest)) (*esapi.Response, error)

type ESUpdate func(index []string, o ...func(*esapi.UpdateByQueryRequest)) (*esapi.Response, error)

type ESDelete func(index []string, body io.Reader, o ...func(*esapi.DeleteByQueryRequest)) (*esapi.Response, error)

type ESCreateIndex func(index string, o ...func(*esapi.IndicesCreateRequest)) (*esapi.Response, error)

type ESDeleteIndex func(index []string, o ...func(*esapi.IndicesDeleteRequest)) (*esapi.Response, error)

type ESCount func(o ...func(*esapi.CountRequest)) (*esapi.Response, error)

// Init returns an implementation of IConnector that interacts with Elasticsearch.
func Init(bc base.Component) *component {
	once.Do(func() {
		comp = &component{Component: bc}
		err := comp.LoadDefinition(definitionJSON, setupJSON, tasksJSON, nil)
		if err != nil {
			panic(err)
		}
	})

	return comp
}

func (c *component) CreateExecution(sysVars map[string]any, setup *structpb.Struct, task string) (*base.ExecutionWrapper, error) {
	e := &execution{
		ComponentExecution: base.ComponentExecution{Component: c, SystemVariables: sysVars, Setup: setup, Task: task},
		client:             *newClient(setup),
	}

	switch task {
	case TaskVectorSearch:
		e.execute = e.search
	case TaskSearch:
		e.execute = e.search
	case TaskIndex:
		e.execute = e.index
	case TaskUpdate:
		e.execute = e.update
	case TaskDelete:
		e.execute = e.delete
	case TaskCreateIndex:
		e.execute = e.createIndex
	case TaskDeleteIndex:
		e.execute = e.deleteIndex
	default:
		return nil, errmsg.AddMessage(
			fmt.Errorf("not supported task: %s", task),
			fmt.Sprintf("%s task is not supported.", task),
		)
	}

	return &base.ExecutionWrapper{Execution: e}, nil
}

func (e *execution) Execute(_ context.Context, inputs []*structpb.Struct) ([]*structpb.Struct, error) {
	outputs := make([]*structpb.Struct, len(inputs))

	for i, input := range inputs {
		output, err := e.execute(input)
		if err != nil {
			return nil, err
		}

		outputs[i] = output
	}

	return outputs, nil
}
