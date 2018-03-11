package dynaGo

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
)

var provider = credentials.StaticProvider{
	credentials.Value{
		AccessKeyID:     "id",
		SecretAccessKey: "secret",
		ProviderName:    "testProvider",
	},
}

var (
	tn = TableName(KeysTestUsrType)
	in = "EmailIndex"
	fn = "Email"
	db = dynamodb.New(
		session.New(
			&aws.Config{
				Credentials: credentials.NewCredentials(&provider),
				Endpoint:    aws.String("http://localhost:8000"),
				Region:      aws.String("us-east-1"),
			}))
)

func TestMain(m *testing.M) {
	//SETUP - try to build necessary tables
	createTables()
	os.Exit(m.Run())
}
func TestSecondaryIndexCreate(t *testing.T) {
	if testTableHasIndex(tn, in, t) != nil {
		deleteSecondaryIndex(KeysTestUsrType, in)
		t.Errorf("Secondary Index %s exists when it should not!", in)
	}
	uti, err := createSecondaryIndex(KeysTestUsrType, key{t: secGbl, pkn: fn})
	if err != nil {
		t.Errorf("could not create secondaryIndex %s:: %s", fn, err)
		t.FailNow()
	}

	_, err = db.UpdateTable(&uti)
	if err != nil {
		t.Errorf("could not update table with secondary index %s:: %s", in, err)
		t.FailNow()
	}
	desc := testTableHasIndex(tn, in, t)
	//Don't need to test name because found by name
	matches := false
	for _, a := range desc.KeySchema {
		if aws.StringValue(a.AttributeName) == fn {
			if aws.StringValue(a.KeyType) == "HASH" {
				matches = true
			}
		}
	}
	if !matches {
		t.Errorf("Index schema did not match intended value")
	}
}
func TestSecondaryIndexDelete(t *testing.T) {
	if testTableHasIndex(tn, in, t) == nil {
		t.Errorf("%s table does not have index '%s' to delete!", tn, in)
	}
	uti, err := deleteSecondaryIndex(KeysTestUsrType, in)
	_, err = db.UpdateTable(&uti)
	if err != nil {
		t.Errorf("Delete '%s' from '%s' failed:: %s", in, tn, err)
	}
	if testTableHasIndex(tn, in, t) != nil {
		t.Errorf("Index '%s' continues to exist after deleted from table '%s'", in, tn)
	}
}

func TestSecondaryIndexCreateKeyMaker(t *testing.T) {
	em := "bob@vila.hmi"
	dto, err := db.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tn)})
	if err != nil {
		t.Errorf("Describe table query failed: %s", err)
		t.FailNow()
	}
	km := CreateKeyMakerByName(KeysTestUsrType, in, *dto)
	key, err := km(em)
	if err != nil {
		t.Errorf("KeyMaker failed to construct key:: %s", err)
	}
	if key.t != secGbl {
		t.Errorf("KeyMaker produced key of type '%d', expected '%d'", key.t, secGbl)
	}
	if key.pkn != fn {
		t.Errorf("KeyMaker produced key with partition key '%s', expected '%s'", key.pkn, fn)
	}
	if key.rkn != "" {
		t.Errorf("KeyMaker produced key with range key '%s', expected '%s'", key.rkn, fn)
	}
	if key.tbln != tn {
		t.Errorf("KeyMaker produced key for table '%s', expected '%s'", key.tbln, tn)
	}
	if key.attr == nil || key.attr[fn] == nil || aws.StringValue(key.attr[fn].S) != em {
		t.Error("KeyMaker produced incorrect attribute value map.")
	}
}

// UTILITIES

func testTableHasIndex(tn string, in string, t *testing.T) *dynamodb.GlobalSecondaryIndexDescription {
	all, err := db.DescribeTable(&dynamodb.DescribeTableInput{TableName: aws.String(tn)})
	if err != nil {
		t.Errorf("Describe Table failed: %s", err)
	}
	for _, a := range all.Table.GlobalSecondaryIndexes {
		if aws.StringValue(a.IndexName) == in {
			return a
		}
	}
	return nil
}

func testTableExists(name string, t *testing.T) bool {
	all, err := db.ListTables(&dynamodb.ListTablesInput{})
	if err != nil {
		t.Errorf("ListTables failed: %s", err)
	}
	for _, a := range all.TableNames {
		if aws.StringValue(a) == name {
			return true
		}
	}
	return false
}

// EXAMPLE TYPES

type KeysTestUsr struct {
	ID    string `dynaGo:"UserID,HASH"`
	Phone string
	Email string
}

type KeysTestMsg struct {
	MID       string `dynaGo:"MessageID"`
	RID       string `dynaGo:"RouteID,HASH"`
	Timestamp int64  `dyanGo:",RANGE"`
}

// these are just easier to use than TypeOf() everywhere - reflect.Type
var (
	KeysTestUsrType = reflect.TypeOf((*KeysTestUsr)(nil)).Elem()
	KeysTestMsgType = reflect.TypeOf((*KeysTestMsg)(nil)).Elem()
)

func createTables() {
	fmt.Printf("CreateTable %s\n", TableName(KeysTestUsrType))
	if err := CreateTable(db, KeysTestUsr{}, 1, 1); err != nil {
		fmt.Printf("\t error occured while creating table: %s\n", err)
	}
	fmt.Printf("CreateTable %s\n", TableName(KeysTestMsgType))
	if err := CreateTable(db, KeysTestMsg{}, 1, 1); err != nil {
		fmt.Printf("\t WARN: error occured while creating table: %s\n", err)
	}
}
