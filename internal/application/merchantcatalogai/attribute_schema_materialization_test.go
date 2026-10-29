package merchantcatalogai

import (
	"context"
	"testing"
	"time"

	"github.com/Ammar777782439/mujeeb24-backend-go/internal/application/ports"
)

type testAttributeCatalog struct {
	ports.CatalogRepository
	created ports.AttributeSchemaDraft
	next int
}
func (s *testAttributeCatalog) NextAttributeSchemaVersion(_ context.Context, _, _ string) (int,error) {return s.next,nil}
func (s *testAttributeCatalog) CreateAttributeSchemaVersion(_ context.Context, in ports.AttributeSchemaDraft)(ports.AttributeSchemaRecord,error){
	s.created=in
	out:=ports.AttributeSchemaRecord{ID:in.ID, BusinessID:in.BusinessID,Name:in.Name,Version:in.Version}
	for _,d:=range in.Definitions {out.Definitions=append(out.Definitions,ports.AttributeDefinitionRecord{
		ID:d.ID,Key:d.Key,Label:d.Label,DataType:d.DataType,Required:d.Required,ValidationRules:d.ValidationRules,DisplayOrder:d.DisplayOrder,
	})}
	return out,nil
}

func TestMaterializeArabicAttributeDefinitionsAndKeyMapping(t *testing.T) {
	stub:=&testAttributeCatalog{next:1}
	specs:=[]ProposedAttributeDefinition{
		{Key:"warranty",Label:"الضمان",DataType:"text"},
		{Key:"battery_health",Label:"صحة البطارية",DataType:"number"},
	}
	record,mapping,err:=resolveApprovedAttributeDefinitions(context.Background(),stub,"business","proposal",nil,specs,time.Now().UTC())
	if err!=nil{t.Fatal(err)}
	if record==nil || record.ID=="" ||record.Version!=1 ||len(stub.created.Definitions)!=2{t.Fatalf("schema not created: %+v",record)}
	for _,s:=range specs{
		key:=mapping[s.Key]
		if len(key)<10 || key[:5]!="attr_" {t.Fatalf("missing server-issued stable key for %q: %q",s.Key,key)}
	}
	attrs,err:=remapApprovedAttributes(map[string]any{"warranty":"سنتان","battery_health":99},mapping)
	if err!=nil{t.Fatal(err)}
	if attrs[mapping["warranty"]]!="سنتان" || attrs[mapping["battery_health"]]!=99 {
		t.Fatalf("source values not preserved: %+v",attrs)
	}
}

func TestExtendVersionedSchemaPreservesManagedAttributeKey(t *testing.T) {
	stub:=&testAttributeCatalog{next:3}
	old:=&ports.AttributeSchemaRecord{ID:"old-schema",BusinessID:"business",Name:"مواصفات الهواتف",Version:2,
		Definitions:[]ports.AttributeDefinitionRecord{{ID:"old-definition",Key:"attr_0123456789abcdef",Label:"اللون",DataType:"text",Required:true}}}
	record,mapping,err:=resolveApprovedAttributeDefinitions(context.Background(),stub,"business","proposal",old,[]ProposedAttributeDefinition{
		{Key:"color",Label:"اللون",DataType:"text"},
		{Key:"warranty",Label:"الضمان",DataType:"text"},
	},time.Now().UTC())
	if err!=nil {t.Fatal(err)}
	if record.Version!=3 || record.ID==old.ID {t.Fatalf("new schema version not created: %+v",record)}
	if mapping["color"]!=old.Definitions[0].Key {t.Fatalf("existing attribute key not reused: %v",mapping)}
	if len(stub.created.Definitions)!=2 || stub.created.Definitions[0].Key!=old.Definitions[0].Key {
		t.Fatalf("existing definitions not preserved across schema versions: %+v",stub.created.Definitions)
	}
	if mapping["warranty"]==mapping["color"] {t.Fatal("unrelated definitions aliased")}
}

func TestMerchantAttributeLabelsRejectUnapprovedMissingOrEnglishLabel(t *testing.T) {
	base:=Proposal{SchemaVersion:ProposalSchemaVersion,Status:StatusResolved,Operation:OperationUpdate,
		ResponseText:"مراجعة التعديل",Update:&UpdateOperation{
			ItemID:"product",Changes:ItemChanges{Attributes:map[string]any{"warranty":"سنتان"}},
		},
	}
	if err:=base.ValidateAttributeDefinitions(true);err==nil{t.Fatal("missing Arabic label accepted")}
	base.AttributeDefinitions=[]ProposedAttributeDefinition{{Key:"warranty",Label:"Warranty",DataType:"text"}}
	if err:=base.ValidateAttributeDefinitions(true);err==nil{t.Fatal("English-only label accepted")}
	base.AttributeDefinitions[0].Label="الضمان"
	if err:=base.ValidateAttributeDefinitions(true);err!=nil{t.Fatal(err)}
}
