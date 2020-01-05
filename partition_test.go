package tengo

import (
	"fmt"
	"strings"
	"testing"
)

func TestTableCreatePartitioning(t *testing.T) {
	unpartitioned := unpartitionedTable(FlavorUnknown)
	partitioned := partitionedTable(FlavorUnknown)
	s1 := aSchema("s1")
	s2 := aSchema("s2", &partitioned)
	sd := NewSchemaDiff(&s1, &s2)
	if len(sd.TableDiffs) != 1 {
		t.Fatalf("Incorrect number of table diffs: expected 1, found %d", len(sd.TableDiffs))
	}
	td := sd.TableDiffs[0]
	if td.DiffType() != DiffTypeCreate || td.Type.String() != "CREATE" {
		t.Fatalf("Incorrect type of table diff returned: expected %s, found %s", DiffTypeCreate, td.Type)
	}

	mods := StatementModifiers{}
	expected := partitioned.CreateStatement
	if actual, err := td.Statement(mods); err != nil {
		t.Fatalf("Unexpected error from Statement: %+v", err)
	} else if actual != expected {
		t.Errorf("Unexpected return from Statement: expected %q, found %q", expected, actual)
	}

	mods.Partitioning = PartitioningRemove
	expected = unpartitioned.CreateStatement
	if actual, err := td.Statement(mods); err != nil {
		t.Fatalf("Unexpected error from Statement: %+v", err)
	} else if actual != expected {
		t.Errorf("Unexpected return from Statement: expected %q, found %q", expected, actual)
	}
}

func TestTableAlterPartitioningStatus(t *testing.T) {
	unpartitioned := unpartitionedTable(FlavorUnknown)
	partitioned := partitionedTable(FlavorUnknown)

	tableAlters, supported := unpartitioned.Diff(&partitioned)
	if !supported {
		t.Error("ALTER to add partitioning unexpectedly unsupported")
	} else if len(tableAlters) != 1 {
		t.Errorf("Wrong number of alter clauses: expected 1, found %d: %+v", len(tableAlters), tableAlters)
	} else if clause, ok := tableAlters[0].(PartitionBy); !ok {
		t.Errorf("Wrong type of alter clause: expected %T, found %T", clause, tableAlters[0])
	} else {
		mods := StatementModifiers{}
		if expected, actual := strings.TrimSpace(partitioned.Partitioning.Definition(FlavorUnknown)), clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
		mods.Partitioning = PartitioningRemove
		if expected, actual := "", clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
	}

	tableAlters, supported = partitioned.Diff(&unpartitioned)
	if !supported {
		t.Error("ALTER to remove partitioning unexpectedly unsupported")
	} else if len(tableAlters) != 1 {
		t.Errorf("Wrong number of alter clauses: expected 1, found %d: %+v", len(tableAlters), tableAlters)
	} else if clause, ok := tableAlters[0].(RemovePartitioning); !ok {
		t.Errorf("Wrong type of alter clause: expected %T, found %T", clause, tableAlters[0])
	} else {
		mods := StatementModifiers{}
		if expected, actual := "REMOVE PARTITIONING", clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
		mods.Partitioning = PartitioningKeep
		if expected, actual := "", clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
	}

	repartitioned := partitionedTable(FlavorUnknown)
	repartitioned.Partitioning.Expression = strings.Replace(repartitioned.Partitioning.Expression, "customer_", "", 1)
	repartitioned.CreateStatement = repartitioned.GeneratedCreateStatement(FlavorUnknown)
	tableAlters, supported = partitioned.Diff(&repartitioned)
	if !supported {
		t.Error("ALTER to change partitioning expression unexpectedly unsupported")
	} else if len(tableAlters) != 1 {
		t.Errorf("Wrong number of alter clauses: expected 1, found %d: %+v", len(tableAlters), tableAlters)
	} else if clause, ok := tableAlters[0].(PartitionBy); !ok {
		t.Errorf("Wrong type of alter clause: expected %T, found %T", clause, tableAlters[0])
	} else {
		mods := StatementModifiers{Partitioning: PartitioningKeep}
		if expected, actual := "", clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
		mods.Partitioning = PartitioningRemove
		if expected, actual := "", clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
		mods.Partitioning = PartitioningPermissive
		if expected, actual := strings.TrimSpace(repartitioned.Partitioning.Definition(FlavorUnknown)), clause.Clause(mods); expected != actual {
			t.Errorf("Unexpected return from Clause(): expected %q, found %q", expected, actual)
		}
	}
}

func TestTableAlterPartitioningOther(t *testing.T) {
	assertIgnored := func(t1, t2 *Table) {
		t.Helper()
		t2.CreateStatement = "" // bypass diff logic short-circuit on matching CreateStatement
		tableAlters, supported := t1.Diff(t2)
		if !supported || len(tableAlters) != 1 {
			t.Errorf("Unexpected return from Diff: %d alters / %t supported", len(tableAlters), supported)
		} else {
			_, ok := tableAlters[0].(ModifyPartitions)
			clause := tableAlters[0].Clause(StatementModifiers{})
			if !ok || clause != "" {
				t.Errorf("Unexpected type or clause returned from diff: %T %s", tableAlters[0], clause)
			}
		}
	}

	assertUnsupported := func(t1, t2 *Table) {
		t.Helper()
		t2.CreateStatement = "" // bypass diff logic short-circuit on matching CreateStatement
		_, supported := t1.Diff(t2)
		if supported {
			t.Error("Expected diff to be unsupported, but it was supported")
		}
	}

	// Changes to the partition list are ignored (via placeholder
	// ModifyPartitions clause) for unit test table since it has RANGE partitioning
	p1, p2 := partitionedTable(FlavorUnknown), partitionedTable(FlavorUnknown)
	p2.Partitioning.Partitions[1].Comment = "hello world"
	assertIgnored(&p1, &p2)
	p2.Partitioning.Partitions = []*Partition{p2.Partitioning.Partitions[0], p2.Partitioning.Partitions[2]}
	assertIgnored(&p1, &p2)
	assertIgnored(&p2, &p1)

	// Changes to the partition list are unsupported for HASH partitioning
	p1.Partitioning.Method, p2.Partitioning.Method = "HASH", "HASH"
	assertUnsupported(&p1, &p2)
	assertUnsupported(&p2, &p1)
}

func TestTableUnpartitionedCreateStatement(t *testing.T) {
	flavors := []Flavor{FlavorMySQL55, FlavorPercona56, FlavorMySQL80, FlavorMariaDB102}
	for _, flavor := range flavors {
		unpartitioned := unpartitionedTable(flavor)
		partitioned := partitionedTable(flavor)
		partitioned.UnsupportedDDL = false
		expected, actual := unpartitioned.CreateStatement, partitioned.UnpartitionedCreateStatement(flavor)
		if actual != expected {
			t.Errorf("Unexpected return from UnpartitionedCreateStatement(%s): expected %q, found %q", flavor, expected, actual)
		}
		_, actualPartClause := ParseCreatePartitioning(partitioned.CreateStatement)
		expectedPartClause := partitioned.Partitioning.Definition(flavor)
		if actualPartClause != expectedPartClause {
			t.Errorf("Unexpected 2nd return val from ParseCreatePartitioning with %s: expected %q, found %q", flavor, expectedPartClause, actualPartClause)
		}

		// Test separate code path for supplying FlavorUnknown to UnpartitionedCreateStatement
		if actual := partitioned.UnpartitionedCreateStatement(FlavorUnknown); actual != expected {
			t.Errorf("Unexpected return from UnpartitionedCreateStatement(FlavorUnknown): expected %q, found %q", expected, actual)
		}

		// Confirm correct return value for already-unpartitioned table
		if actual := unpartitioned.UnpartitionedCreateStatement(flavor); actual != expected {
			t.Errorf("Unexpected return from UnpartitionedCreateStatement(%s): expected %q, found %q", flavor, expected, actual)
		}
		if base, partClause := ParseCreatePartitioning(unpartitioned.CreateStatement); base != unpartitioned.CreateStatement || partClause != "" {
			t.Errorf("Unexpected return from ParseCreatePartitioning on unpartitioned table: returned %q, %q", base, partClause)
		}
	}
}

func TestSchemaDiffDropPartitionedTable(t *testing.T) {
	table := partitionedTable(FlavorUnknown)
	s1 := aSchema("s1", &table)
	s2 := aSchema("s2")

	// Expectation: this diff should contain ALTERs to drop 2 out of the 3
	// partitions in table, and then a DROP TABLE for the table.
	diff := NewSchemaDiff(&s1, &s2)
	expectStatements := []string{
		fmt.Sprintf("ALTER TABLE %s DROP PARTITION %s", EscapeIdentifier(table.Name), table.Partitioning.Partitions[0].Name),
		fmt.Sprintf("ALTER TABLE %s DROP PARTITION %s", EscapeIdentifier(table.Name), table.Partitioning.Partitions[1].Name),
		fmt.Sprintf("DROP TABLE %s", EscapeIdentifier(table.Name)),
	}
	objDiffs := diff.ObjectDiffs()
	if len(objDiffs) != len(expectStatements) {
		t.Errorf("Expected %d statements, instead found %d", len(expectStatements), len(objDiffs))
	} else {
		for n, od := range objDiffs {
			stmt, err := od.Statement(StatementModifiers{LockClause: "SHARED", AlgorithmClause: "COPY"})
			if stmt != expectStatements[n] {
				t.Errorf("Statement[%d]: Expected %q, found %q", n, expectStatements[n], stmt)
			}
			if !IsForbiddenDiff(err) {
				t.Errorf("Statement[%d]: Expected forbidden diff error, instead err=%v", n, err)
			}
			if _, err = od.Statement(StatementModifiers{AllowUnsafe: true}); err != nil {
				t.Errorf("Statement[%d]: Expected no error with AllowUnsafe enabled, instead found err=%v", n, err)
			}
			stmt, _ = od.Statement(StatementModifiers{SkipPreDropAlters: true})
			var expected string
			if !strings.HasPrefix(expectStatements[n], "ALTER") {
				expected = expectStatements[n]
			}
			if stmt != expected {
				t.Errorf("Statement[%d]: With SkipPreDropAlters, expected %q but found %q", n, expected, stmt)
			}
		}
	}

	// After changing the partitioning type to one that doesn't support ALTER
	// TABLE ... DROP PARTITION, a diff should only contain the DROP TABLE.
	table.Partitioning.Method = "HASH"
	diff = NewSchemaDiff(&s1, &s2)
	expectStatements = expectStatements[2:]
	objDiffs = diff.ObjectDiffs()
	if len(objDiffs) != len(expectStatements) {
		t.Errorf("Expected %d statements, instead found %d", len(expectStatements), len(objDiffs))
	} else {
		stmt, _ := objDiffs[0].Statement(StatementModifiers{})
		if stmt != expectStatements[0] {
			t.Errorf("Statement[0]: Expected %q, found %q", expectStatements[0], stmt)
		}
	}
}

// TestPartitioningDataDirectory handles the chunk of code in
// fixPartitioningEdgeCases relating to data directory parsing. This isn't
// handled by integration tests due to complexity of setup in containers.
func TestPartitioningDataDirectory(t *testing.T) {
	table := partitionedTable(FlavorUnknown)
	table.CreateStatement = strings.Replace(table.CreateStatement, "LESS THAN (123)", "LESS THAN (123) DATA DIRECTORY = '/some/weird/dir'", 1)
	table.CreateStatement = strings.Replace(table.CreateStatement, "LESS THAN MAXVALUE", "LESS THAN MAXVALUE DATA DIRECTORY = '/some/weirder/dir'", 1)
	if table.CreateStatement == table.GeneratedCreateStatement(FlavorUnknown) {
		t.Fatal("Failed to set up test properly: string replacements did not match")
	}
	fixPartitioningEdgeCases(&table, FlavorUnknown)
	if table.CreateStatement != table.GeneratedCreateStatement(FlavorUnknown) {
		t.Errorf("Failed to extract data directories; post-fix partitioning statement generated as %s", table.Partitioning.Definition(FlavorUnknown))
	}
}

func (s TengoIntegrationSuite) TestPartitionedIntrospection(t *testing.T) {
	if _, err := s.d.SourceSQL("testdata/partition.sql"); err != nil {
		t.Fatalf("Unexpected error sourcing testdata/partition.sql: %v", err)
	}
	schema := s.GetSchema(t, "partitionparty")
	flavor := s.d.Flavor()

	// Ensure our unit test fixture and integration test fixture match
	tableFromDB := schema.Table("prange")
	tableFromUnit := partitionedTable(flavor)
	tableFromUnit.CreateStatement = "" // Prevent diff from short-circuiting on equivalent CREATEs
	clauses, supported := tableFromDB.Diff(&tableFromUnit)
	if !supported {
		t.Error("Diff unexpectedly not supported for unit test partitioned table")
	} else if len(clauses) > 0 {
		t.Errorf("Diff of partitioned table unexpectedly found %d clauses; expected 0. Clauses: %+v", len(clauses), clauses)
	}

	// ensure partitioned tables are introspected correctly by confirming that
	// they are supported for diffs. Additionally confirm that
	// UnpartitionedCreateStatement returns the expected value.
	for _, table := range schema.Tables {
		if table.UnsupportedDDL {
			t.Errorf("Table %s unexpectedly has UnsupportedDDL==true\nExpected SHOW CREATE TABLE:\n%s\nActual SHOW CREATE TABLE:\n%s", table.Name, table.GeneratedCreateStatement(flavor), table.CreateStatement)
		}
		actual := table.UnpartitionedCreateStatement(flavor)
		table.Partitioning = nil
		expected := table.GeneratedCreateStatement(flavor)
		if actual != expected {
			t.Errorf("Table %s unexpected result from UnpartitionedCreateStatement: expected %q, found %q", table.Name, expected, actual)
		}
	}
}

func (s TengoIntegrationSuite) TestDropPartitionedTable(t *testing.T) {
	if _, err := s.d.SourceSQL("testdata/partition.sql"); err != nil {
		t.Fatalf("Unexpected error sourcing testdata/partition.sql: %v", err)
	}

	// Setup: build a "to" schema which removes 2 tables in the "from" schema:
	// one partitioned using RANGE COLUMNS and one partitioned using LINEAR KEY
	from := s.GetSchema(t, "partitionparty")
	to := s.GetSchema(t, "partitionparty")
	var keepTables []*Table
	for _, table := range to.Tables {
		if table.Name != "prangecol" && table.Name != "plinearkey" {
			keepTables = append(keepTables, table)
		}
	}
	to.Tables = keepTables
	if len(to.Tables) != len(from.Tables)-2 {
		t.Fatal("Fatal problem in test setup: table names from partition.sql have changed?")
	}

	// Confirm diff contains expected number of statements
	diff := NewSchemaDiff(from, to)
	objDiffs := diff.ObjectDiffs()
	expectLen := len(from.Table("prangecol").Partitioning.Partitions) + 1
	if len(objDiffs) != expectLen {
		t.Errorf("Expected %d ObjectDiffs, instead found %d", expectLen, len(objDiffs))
	}

	// Execute the statements to confirm they are syntactically valid and in the
	// correct order (e.g. ALTERs to drop partitions come before DROP TABLE)
	db, err := s.d.Connect("partitionparty", "")
	if err != nil {
		t.Fatalf("Unable to connect to db: %v", err)
	}
	mods := StatementModifiers{
		AllowUnsafe:     true,     // permit the DROPs
		LockClause:      "SHARED", // confirming this is removed for DROP PARTITION
		AlgorithmClause: "COPY",   // ditto
	}
	for _, od := range objDiffs {
		stmt, err := od.Statement(mods)
		if err != nil {
			t.Errorf("Unexpected error from Statement: %v", err)
		} else if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("Unexpected error running statement %q: %v", stmt, err)
		}
	}

	// Confirm the statements had the intended effect
	after := s.GetSchema(t, "partitionparty")
	diff = NewSchemaDiff(after, to)
	objDiffs = diff.ObjectDiffs()
	if len(objDiffs) != 0 {
		t.Errorf("Expected no remaining diffs, instead found %d", len(objDiffs))
	}
}

func (s TengoIntegrationSuite) TestBulkDropPartitioned(t *testing.T) {
	if _, err := s.d.SourceSQL("testdata/partition.sql"); err != nil {
		t.Fatalf("Unexpected error sourcing testdata/partition.sql: %v", err)
	}
	opts := BulkDropOptions{
		MaxConcurrency:  15,
		PartitionsFirst: true,
	}
	err := s.d.DropTablesInSchema("partitionparty", opts)
	if err != nil {
		t.Errorf("Unexpected error from DropTablesInSchema: %v", err)
	}
}

func (s TengoIntegrationSuite) TestAlterPartitioning(t *testing.T) {
	if _, err := s.d.SourceSQL("testdata/partition.sql"); err != nil {
		t.Fatalf("Unexpected error sourcing testdata/partition.sql: %v", err)
	}

	flavor := s.d.Flavor()
	mods := StatementModifiers{AllowUnsafe: true, Flavor: flavor}
	tableFromDB := s.GetTable(t, "partitionparty", "prange")
	tableFromUnit := unpartitionedTable(flavor)
	tableFromUnitP := partitionedTable(flavor)
	if tableFromDB.CreateStatement != tableFromUnitP.CreateStatement {
		t.Fatalf("Test requires no drift between definition of unit test table and corresponding actual table; found %q vs %q", tableFromDB.CreateStatement, tableFromUnitP.CreateStatement)
	}
	db, err := s.d.Connect("partitionparty", "")
	if err != nil {
		t.Fatalf("Unable to connect to DockerizedInstance: %v", err)
	}

	// Confirm that combining REMOVE PARTITIONING with other clauses works
	// properly, since the syntax is unusual  (no comma before partitioning clause)
	tableFromUnit.Columns = append(tableFromUnit.Columns,
		&Column{
			Name:     "foo1",
			TypeInDB: "int(10) unsigned",
		},
	)
	tableFromUnit.CreateStatement = tableFromUnit.GeneratedCreateStatement(flavor)
	stmt, _ := NewAlterTable(tableFromDB, &tableFromUnit).Statement(mods)
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("Unexpected error running statement %q: %v", stmt, err)
	}
	tableFromDB = s.GetTable(t, "partitionparty", "prange")
	if tableFromDB.Partitioning != nil || len(tableFromDB.Columns) != len(tableFromUnit.Columns) {
		t.Fatalf("Statement %q did not have the intended effect", stmt)
	}

	// Now confirm combining PARTITION BY with other clauses works properly,
	// again because the syntax is unusual (no comma before partitioning clause)
	stmt, _ = NewAlterTable(tableFromDB, &tableFromUnitP).Statement(mods)
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("Unexpected error running statement %q: %v", stmt, err)
	}
	tableFromDB = s.GetTable(t, "partitionparty", "prange")
	if tableFromDB.CreateStatement != tableFromUnitP.CreateStatement {
		t.Fatalf("Statement %q did not have the intended effect", stmt)
	}

	// Ditto but this time we're changing the partitioning expression
	tableFromUnitP.Columns = append(tableFromUnitP.Columns,
		&Column{
			Name:     "foo2",
			TypeInDB: "int(10) unsigned",
		},
	)
	tableFromUnitP.Partitioning.Expression = strings.Replace(tableFromUnitP.Partitioning.Expression, "customer_", "", 1)
	tableFromUnitP.CreateStatement = tableFromUnitP.GeneratedCreateStatement(flavor)
	stmt, _ = NewAlterTable(tableFromDB, &tableFromUnitP).Statement(mods)
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("Unexpected error running statement %q: %v", stmt, err)
	}
	tableFromDB = s.GetTable(t, "partitionparty", "prange")
	if tableFromDB.CreateStatement != tableFromUnitP.CreateStatement {
		t.Fatalf("Statement %q did not have the intended effect", stmt)
	}
}

// Keep this definition in sync with table prange in partition.sql
func partitionedTable(flavor Flavor) Table {
	t := unpartitionedTable(flavor)
	expression := "customer_id"
	if flavor.HasDataDictionary() || flavor.VendorMinVersion(VendorMariaDB, 10, 2) {
		expression = EscapeIdentifier(expression)
	}
	t.Partitioning = &TablePartitioning{
		Method:     "RANGE",
		Expression: expression,
		Partitions: []*Partition{
			{Name: "p0", Values: "123", Engine: "InnoDB"},
			{Name: "p1", Values: "456", Engine: "InnoDB"},
			{Name: "p2", Values: "MAXVALUE", Engine: "InnoDB"},
		},
	}
	t.CreateStatement = t.GeneratedCreateStatement(flavor)
	return t
}

func unpartitionedTable(flavor Flavor) Table {
	columns := []*Column{
		{
			Name:          "id",
			TypeInDB:      "int(10) unsigned",
			AutoIncrement: true,
		},
		{
			Name:     "customer_id",
			TypeInDB: "int(10) unsigned",
		},
		{
			Name:               "info",
			TypeInDB:           "text",
			Nullable:           true,
			CharSet:            "latin1",
			Collation:          "latin1_swedish_ci",
			CollationIsDefault: true,
		},
	}
	if flavor.AllowBlobDefaults() {
		columns[2].Default = "NULL"
	}
	t := Table{
		Name:               "prange",
		Engine:             "InnoDB",
		CharSet:            "latin1",
		Collation:          "latin1_swedish_ci",
		CollationIsDefault: true,
		CreateOptions:      "ROW_FORMAT=REDUNDANT",
		Columns:            columns,
		PrimaryKey:         primaryKey(columns[0], columns[1]),
		SecondaryIndexes:   []*Index{},
		ForeignKeys:        []*ForeignKey{},
		NextAutoIncrement:  1,
	}
	t.CreateStatement = t.GeneratedCreateStatement(flavor)
	return t
}
