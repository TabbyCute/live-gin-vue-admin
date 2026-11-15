package testlocal

import "testing"

func TestEventDeliveryMySQLRejectsUnsafeTargets(t *testing.T) {
	for _, dsn := range []string{"root@tcp(db.example.com:3306)/event_test_delivery", "root@tcp(127.0.0.1:13329)/production", "root@tcp(127.0.0.1:3306)/event_test_delivery", "root@tcp(localhost:13329)/event_test_delivery", "root@unix(/tmp/mysql.sock)/event_test_delivery", "invalid"} {
		if _, err := EventDeliveryMySQL(dsn); err == nil {
			t.Fatal("unsafe target accepted")
		}
	}
	if _, err := EventDeliveryMySQL("root:local-test@tcp(127.0.0.1:13329)/event_test_delivery?parseTime=true"); err != nil {
		t.Fatal(err)
	}
}
