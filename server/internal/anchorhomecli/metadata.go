package anchorhomecli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"tb_live_module/initialize"
)

type metadataProof struct {
	Rows    map[string]map[string]int
	Allowed map[string]map[string]bool
}

// Preserve the exact full row multiset, including unknown future columns and
// duplicate join rows. sys_authority_menus need not have a primary key.
func metadataInSnapshot(tx *gorm.DB) (metadataProof, error) {
	out := metadataProof{Rows: map[string]map[string]int{}, Allowed: map[string]map[string]bool{}}
	type record struct {
		table, digest string
		fields        map[string]string
	}
	var records []record
	var moduleID, parentID string
	for _, table := range []string{"sys_base_menus", "sys_authority_menus", "sys_apis"} {
		rows, err := tx.Raw("SELECT * FROM `" + table + "`").Rows()
		if err != nil {
			return out, err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Rows[table], out.Allowed[table] = map[string]int{}, map[string]bool{}
		for rows.Next() {
			values, destinations := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err = rows.Scan(destinations...); err != nil {
				rows.Close()
				return out, err
			}
			data, err := json.Marshal(values)
			if err != nil {
				rows.Close()
				return out, err
			}
			digest := sha256.Sum256(data)
			r := record{table: table, digest: hex.EncodeToString(digest[:]), fields: map[string]string{}}
			for i, value := range values {
				if value == nil {
					r.fields[columns[i]] = ""
				} else if raw, ok := value.([]byte); ok {
					r.fields[columns[i]] = string(raw)
				} else {
					r.fields[columns[i]] = fmt.Sprint(value)
				}
			}
			if table == "sys_base_menus" && r.fields["deleted_at"] == "" {
				if r.fields["name"] == initialize.AnchorHomeMenuName {
					moduleID = r.fields["id"]
				}
				if r.fields["name"] == "liveManagement" {
					parentID = r.fields["id"]
				}
			}
			out.Rows[table][r.digest]++
			records = append(records, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	for _, r := range records {
		f, allowed := r.fields, false
		switch r.table {
		case "sys_base_menus":
			allowed = parentID != "" && f["name"] == initialize.AnchorHomeMenuName && f["path"] == initialize.AnchorHomeMenuName && f["component"] == "view/live/anchor-home/index.vue" && f["parent_id"] == parentID && f["deleted_at"] == ""
		case "sys_authority_menus":
			allowed = moduleID != "" && f["sys_base_menu_id"] == moduleID && f["sys_authority_authority_id"] == "888"
		case "sys_apis":
			allowed = f["deleted_at"] == "" && (f["path"] == "/v1/admin/anchor-home/album/list" && f["method"] == "POST" || f["path"] == "/v1/admin/anchor-home/album/review" && f["method"] == "POST" || f["path"] == "/v1/admin/anchor-home/album/preview" && f["method"] == "GET")
		}
		out.Allowed[r.table][r.digest] = allowed
	}
	return out, nil
}

func unchangedMetadata(before, after metadataProof, allowNew bool) bool {
	for table, rows := range before.Rows {
		for digest, count := range rows {
			if after.Rows[table][digest] < count {
				return false
			}
		}
	}
	for table, rows := range after.Rows {
		for digest, count := range rows {
			if count > before.Rows[table][digest] && (!allowNew || !after.Allowed[table][digest] || count-before.Rows[table][digest] != 1) {
				return false
			}
		}
	}
	return true
}
