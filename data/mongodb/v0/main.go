//go:generate compogen readme ./config ./README.mdx
package mongodb

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/instill-ai/component/base"
	"github.com/instill-ai/x/errmsg"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	TaskInsert         = "TASK_INSERT"
	TaskFind           = "TASK_FIND"
	TaskUpdate         = "TASK_UPDATE"
	TaskDelete         = "TASK_DELETE"
	TaskDropCollection = "TASK_DROP_COLLECTION"
	TaskDropDatabase   = "TASK_DROP_DATABASE"
)

//go:embed config/definition.json
var definitionJSON []byte

//go:embed config/setup.json
var setupJSON []byte

//go:embed config/tasks.json
var tasksJSON []byte

var once sync.Once
var comp *component

type component struct {
	base.Component
}

type MongoCollectionClient interface {
	Find(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (cur *mongo.Cursor, err error)
	FindOne(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult
	InsertOne(ctx context.Context, document interface{}, opts ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
	UpdateMany(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error)
	DeleteMany(ctx context.Context, filter interface{}, opts ...*options.DeleteOptions) (*mongo.DeleteResult, error)
	Drop(ctx context.Context) error
}

type MongoDatabaseClient interface {
	Drop(ctx context.Context) error
}

type MongoClient struct {
	collectionClient MongoCollectionClient
	databaseClient   MongoDatabaseClient
}

// dbClient for task DropDatabase
// collectionClient for other than task DropDatabase
type execution struct {
	base.ComponentExecution

	execute func(context.Context, *structpb.Struct) (*structpb.Struct, error)
	client  *MongoClient
}

// Init returns an implementation of IConnector that interacts with MongoDB.
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
		client:             &MongoClient{},
	}

	switch task {
	case TaskInsert:
		e.execute = e.insert
	case TaskFind:
		e.execute = e.find
	case TaskUpdate:
		e.execute = e.update
	case TaskDelete:
		e.execute = e.delete
	case TaskDropCollection:
		e.execute = e.dropCollection
	case TaskDropDatabase:
		e.execute = e.dropDatabase
	default:
		return nil, errmsg.AddMessage(
			fmt.Errorf("not supported task: %s", task),
			fmt.Sprintf("%s task is not supported.", task),
		)
	}

	return &base.ExecutionWrapper{Execution: e}, nil
}

type Destination struct {
	DatabaseName   string `json:"database-name"`
	CollectionName string `json:"collection-name"`
}

// dbClient wont be nil on component test (use mock dbClient)
// collectionClient wont be nil on component test (use mock collectionClient)
// collectionClient will be nil on task DropDatabase
func (e *execution) Execute(ctx context.Context, inputs []*structpb.Struct) ([]*structpb.Struct, error) {
	outputs := make([]*structpb.Struct, len(inputs))
	client := newClient(ctx, e.Setup)

	for i, input := range inputs {
		var inputStruct Destination
		err := base.ConvertFromStructpb(input, &inputStruct)
		if err != nil {
			return nil, err
		}

		var db *mongo.Database
		if e.client.databaseClient == nil {
			db = client.Database(inputStruct.DatabaseName)
			e.client.databaseClient = db
		}

		if e.Task != TaskDropDatabase && e.client.collectionClient == nil {
			collection := db.Collection(inputStruct.CollectionName)
			e.client.collectionClient = collection
		}

		output, err := e.execute(ctx, input)
		if err != nil {
			return nil, err
		}

		outputs[i] = output
	}

	return outputs, nil
}
