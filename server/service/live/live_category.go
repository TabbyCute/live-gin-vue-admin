package live

import (
	"errors"
	"regexp"
	"strings"

	"tb_live_module/global"
	liveModel "tb_live_module/model/live"
	liveReq "tb_live_module/model/live/request"
	liveRes "tb_live_module/model/live/response"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrLiveCategoryNotFound       = errors.New("直播分类不存在")
	ErrLiveCategoryCodeInvalid    = errors.New("分类编码必须以小写字母开头，且只能包含小写字母、数字、下划线和短横线")
	ErrLiveCategoryCodeExists     = errors.New("分类编码已存在")
	ErrLiveCategoryNameEmpty      = errors.New("分类名称不能为空")
	ErrLiveCategoryNameExists     = errors.New("同一父分类下已存在同名分类")
	ErrLiveCategoryStatusInvalid  = errors.New("分类状态只能是 0 或 1")
	ErrLiveCategoryParentInvalid  = errors.New("父分类不存在")
	ErrLiveCategoryParentDisabled = errors.New("启用分类的所有上级分类必须处于启用状态")
	ErrLiveCategoryCycle          = errors.New("分类层级不能形成循环")
	ErrLiveCategoryHasChildren    = errors.New("请先删除子分类")
	ErrLiveCategoryInUse          = errors.New("分类已被主播或直播间使用，不能删除")
	ErrLiveCategoryDeleteEnabled  = errors.New("启用中的分类不能删除，请先停用")
)

var liveCategoryCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

type CategoryService struct{}

func (s *CategoryService) GetPublicCategoryTree() ([]liveRes.LiveCategoryPublicItem, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	var categories []liveModel.LiveCategory
	if err := global.GVA_DB.Where("status = ?", liveModel.LiveCategoryStatusEnabled).
		Order("sort DESC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return buildPublicCategoryTree(categories), nil
}

func (s *CategoryService) GetAdminCategoryTree() ([]liveRes.LiveCategoryAdminTreeItem, error) {
	if global.GVA_DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	var categories []liveModel.LiveCategory
	if err := global.GVA_DB.Order("sort DESC, id ASC").Find(&categories).Error; err != nil {
		return nil, err
	}
	return buildAdminCategoryTree(categories), nil
}

func (s *CategoryService) GetAdminCategoryList(req *liveReq.LiveCategoryAdminListReq) ([]liveRes.LiveCategoryAdminItem, int64, error) {
	if global.GVA_DB == nil {
		return nil, 0, errors.New("数据库未初始化")
	}
	db := global.GVA_DB.Model(&liveModel.LiveCategory{})
	keyword := strings.TrimSpace(req.Keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("code LIKE ? OR name LIKE ?", like, like)
	}
	if req.ParentId != nil {
		db = db.Where("parent_id = ?", *req.ParentId)
	}
	if req.Status != nil {
		db = db.Where("status = ?", *req.Status)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var categories []liveModel.LiveCategory
	if err := db.Scopes(req.PageInfo.Paginate()).Order("sort DESC, id ASC").Find(&categories).Error; err != nil {
		return nil, 0, err
	}

	parentIDs := make([]uint, 0)
	seenParentIDs := make(map[uint]struct{})
	for _, category := range categories {
		if category.ParentId == 0 {
			continue
		}
		if _, exists := seenParentIDs[category.ParentId]; exists {
			continue
		}
		seenParentIDs[category.ParentId] = struct{}{}
		parentIDs = append(parentIDs, category.ParentId)
	}
	parentNames := make(map[uint]string, len(parentIDs))
	if len(parentIDs) > 0 {
		var parents []liveModel.LiveCategory
		if err := global.GVA_DB.Select("id", "name").Where("id IN ?", parentIDs).Find(&parents).Error; err != nil {
			return nil, 0, err
		}
		for _, parent := range parents {
			parentNames[parent.ID] = parent.Name
		}
	}

	list := make([]liveRes.LiveCategoryAdminItem, 0, len(categories))
	for _, category := range categories {
		list = append(list, toLiveCategoryAdminItem(category, parentNames[category.ParentId]))
	}
	return list, total, nil
}

func (s *CategoryService) CreateCategory(req liveReq.LiveCategoryCreateReq) (liveRes.LiveCategoryAdminItem, error) {
	if global.GVA_DB == nil {
		return liveRes.LiveCategoryAdminItem{}, errors.New("数据库未初始化")
	}
	if req.Status == nil {
		return liveRes.LiveCategoryAdminItem{}, errors.New("分类状态不能为空")
	}
	if *req.Status != liveModel.LiveCategoryStatusDisabled && *req.Status != liveModel.LiveCategoryStatusEnabled {
		return liveRes.LiveCategoryAdminItem{}, ErrLiveCategoryStatusInvalid
	}
	code := strings.ToLower(strings.TrimSpace(req.Code))
	name := strings.TrimSpace(req.Name)
	if !liveCategoryCodePattern.MatchString(code) {
		return liveRes.LiveCategoryAdminItem{}, ErrLiveCategoryCodeInvalid
	}
	if name == "" {
		return liveRes.LiveCategoryAdminItem{}, ErrLiveCategoryNameEmpty
	}

	category := liveModel.LiveCategory{
		ParentId: req.ParentId,
		Code:     code,
		Name:     name,
		Icon:     strings.TrimSpace(req.Icon),
		Sort:     req.Sort,
		Status:   *req.Status,
	}
	desiredStatus := *req.Status
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		if err := validateLiveCategoryParent(tx, 0, category.ParentId, category.Status); err != nil {
			return err
		}
		if err := ensureLiveCategoryUnique(tx, 0, category.ParentId, category.Code, category.Name); err != nil {
			return err
		}
		if err := tx.Create(&category).Error; err != nil {
			return err
		}
		// GORM 会对带 default:1 标签的零值使用数据库默认值；显式创建停用分类时需要再写回 0。
		if desiredStatus == liveModel.LiveCategoryStatusDisabled {
			if err := tx.Model(&liveModel.LiveCategory{}).Where("id = ?", category.ID).
				Update("status", liveModel.LiveCategoryStatusDisabled).Error; err != nil {
				return err
			}
			category.Status = liveModel.LiveCategoryStatusDisabled
		}
		return nil
	})
	if err != nil {
		if conflict := normalizeLiveCategoryCreateError(code, err); conflict != nil {
			return liveRes.LiveCategoryAdminItem{}, conflict
		}
		return liveRes.LiveCategoryAdminItem{}, err
	}
	parentName, err := getLiveCategoryParentName(global.GVA_DB, category.ParentId)
	if err != nil {
		return liveRes.LiveCategoryAdminItem{}, err
	}
	return toLiveCategoryAdminItem(category, parentName), nil
}

func (s *CategoryService) UpdateCategory(req liveReq.LiveCategoryUpdateReq) (liveRes.LiveCategoryAdminItem, error) {
	if global.GVA_DB == nil {
		return liveRes.LiveCategoryAdminItem{}, errors.New("数据库未初始化")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return liveRes.LiveCategoryAdminItem{}, ErrLiveCategoryNameEmpty
	}
	err := global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		category, err := getLiveCategoryByIDForUpdate(tx, req.ID)
		if err != nil {
			return normalizeLiveCategoryLookupError(err)
		}
		if err = validateLiveCategoryParent(tx, category.ID, req.ParentId, category.Status); err != nil {
			return err
		}
		if err = ensureLiveCategoryUnique(tx, category.ID, req.ParentId, "", name); err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveCategory{}).Where("id = ?", category.ID).Updates(map[string]interface{}{
			"parent_id": req.ParentId,
			"name":      name,
			"icon":      strings.TrimSpace(req.Icon),
			"sort":      req.Sort,
		}).Error
	})
	if err != nil {
		return liveRes.LiveCategoryAdminItem{}, err
	}
	var refreshed liveModel.LiveCategory
	if err = global.GVA_DB.First(&refreshed, req.ID).Error; err != nil {
		return liveRes.LiveCategoryAdminItem{}, normalizeLiveCategoryLookupError(err)
	}
	parentName, err := getLiveCategoryParentName(global.GVA_DB, refreshed.ParentId)
	if err != nil {
		return liveRes.LiveCategoryAdminItem{}, err
	}
	return toLiveCategoryAdminItem(refreshed, parentName), nil
}

func (s *CategoryService) UpdateCategoryStatus(req liveReq.LiveCategoryStatusUpdateReq) error {
	if global.GVA_DB == nil {
		return errors.New("数据库未初始化")
	}
	if req.Status == nil {
		return errors.New("分类状态不能为空")
	}
	if *req.Status != liveModel.LiveCategoryStatusDisabled && *req.Status != liveModel.LiveCategoryStatusEnabled {
		return ErrLiveCategoryStatusInvalid
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		category, err := getLiveCategoryByIDForUpdate(tx, req.ID)
		if err != nil {
			return normalizeLiveCategoryLookupError(err)
		}
		if category.Status == *req.Status {
			return nil
		}
		if *req.Status == liveModel.LiveCategoryStatusEnabled {
			if err = validateLiveCategoryParent(tx, category.ID, category.ParentId, liveModel.LiveCategoryStatusEnabled); err != nil {
				return err
			}
			return tx.Model(&liveModel.LiveCategory{}).Where("id = ?", category.ID).
				Update("status", liveModel.LiveCategoryStatusEnabled).Error
		}

		ids, err := collectLiveCategoryDescendantIDs(tx, category.ID)
		if err != nil {
			return err
		}
		return tx.Model(&liveModel.LiveCategory{}).Where("id IN ?", ids).
			Update("status", liveModel.LiveCategoryStatusDisabled).Error
	})
}

func (s *CategoryService) DeleteCategory(categoryID uint) error {
	if global.GVA_DB == nil {
		return errors.New("数据库未初始化")
	}
	return global.GVA_DB.Transaction(func(tx *gorm.DB) error {
		category, err := getLiveCategoryByIDForUpdate(tx, categoryID)
		if err != nil {
			return normalizeLiveCategoryLookupError(err)
		}
		if category.Status == liveModel.LiveCategoryStatusEnabled {
			return ErrLiveCategoryDeleteEnabled
		}
		var childCount int64
		if err = tx.Model(&liveModel.LiveCategory{}).Where("parent_id = ?", category.ID).Count(&childCount).Error; err != nil {
			return err
		}
		if childCount > 0 {
			return ErrLiveCategoryHasChildren
		}
		var anchorCount int64
		if err = tx.Model(&liveModel.LiveAnchor{}).Where("category_id = ?", category.ID).Count(&anchorCount).Error; err != nil {
			return err
		}
		if anchorCount > 0 {
			return ErrLiveCategoryInUse
		}
		if tx.Migrator().HasTable(&liveModel.LiveRoom{}) {
			var roomCount int64
			if err = tx.Model(&liveModel.LiveRoom{}).Where("category_id = ?", category.ID).Count(&roomCount).Error; err != nil {
				return err
			}
			if roomCount > 0 {
				return ErrLiveCategoryInUse
			}
		}
		return tx.Delete(&liveModel.LiveCategory{}, category.ID).Error
	})
}

// EnsureEnabledCategory 校验 categoryId 是否来自当前启用的直播分类；0 表示清空分类。
func (s *CategoryService) EnsureEnabledCategory(categoryID uint64) error {
	if categoryID == 0 {
		return nil
	}
	if global.GVA_DB == nil {
		return errors.New("数据库未初始化")
	}
	var count int64
	err := global.GVA_DB.Model(&liveModel.LiveCategory{}).
		Where("id = ? AND status = ?", categoryID, liveModel.LiveCategoryStatusEnabled).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrLiveCategoryNotFound
	}
	return nil
}

func validateLiveCategoryParent(db *gorm.DB, categoryID, parentID uint, status uint8) error {
	if parentID == 0 {
		return nil
	}
	visited := map[uint]struct{}{}
	if categoryID > 0 {
		visited[categoryID] = struct{}{}
	}
	currentID := parentID
	for currentID > 0 {
		if _, exists := visited[currentID]; exists {
			return ErrLiveCategoryCycle
		}
		visited[currentID] = struct{}{}
		var parent liveModel.LiveCategory
		if err := db.First(&parent, currentID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrLiveCategoryParentInvalid
			}
			return err
		}
		if status == liveModel.LiveCategoryStatusEnabled && parent.Status != liveModel.LiveCategoryStatusEnabled {
			return ErrLiveCategoryParentDisabled
		}
		currentID = parent.ParentId
	}
	return nil
}

func ensureLiveCategoryUnique(db *gorm.DB, categoryID, parentID uint, code, name string) error {
	if code != "" {
		var count int64
		if err := db.Unscoped().Model(&liveModel.LiveCategory{}).Where("code = ?", code).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrLiveCategoryCodeExists
		}
	}
	query := db.Model(&liveModel.LiveCategory{}).Where("parent_id = ? AND name = ?", parentID, name)
	if categoryID > 0 {
		query = query.Where("id <> ?", categoryID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrLiveCategoryNameExists
	}
	return nil
}

func normalizeLiveCategoryCreateError(code string, createErr error) error {
	if errors.Is(createErr, ErrLiveCategoryCodeExists) || errors.Is(createErr, ErrLiveCategoryNameExists) {
		return createErr
	}
	var count int64
	if global.GVA_DB != nil && global.GVA_DB.Unscoped().Model(&liveModel.LiveCategory{}).Where("code = ?", code).Count(&count).Error == nil && count > 0 {
		return ErrLiveCategoryCodeExists
	}
	return nil
}

func collectLiveCategoryDescendantIDs(db *gorm.DB, rootID uint) ([]uint, error) {
	var categories []liveModel.LiveCategory
	if err := db.Select("id", "parent_id").Find(&categories).Error; err != nil {
		return nil, err
	}
	children := make(map[uint][]uint)
	for _, category := range categories {
		children[category.ParentId] = append(children[category.ParentId], category.ID)
	}
	ids := make([]uint, 0, 1)
	queue := []uint{rootID}
	seen := make(map[uint]struct{})
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		queue = append(queue, children[id]...)
	}
	return ids, nil
}

func getLiveCategoryByIDForUpdate(db *gorm.DB, categoryID uint) (*liveModel.LiveCategory, error) {
	var category liveModel.LiveCategory
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&category, categoryID).Error
	return &category, err
}

func getLiveCategoryParentName(db *gorm.DB, parentID uint) (string, error) {
	if parentID == 0 {
		return "", nil
	}
	var parent liveModel.LiveCategory
	if err := db.Select("id", "name").First(&parent, parentID).Error; err != nil {
		return "", normalizeLiveCategoryLookupError(err)
	}
	return parent.Name, nil
}

func normalizeLiveCategoryLookupError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrLiveCategoryNotFound
	}
	return err
}

func toLiveCategoryAdminItem(category liveModel.LiveCategory, parentName string) liveRes.LiveCategoryAdminItem {
	return liveRes.LiveCategoryAdminItem{
		ID: category.ID, ParentId: category.ParentId, ParentName: parentName,
		Code: category.Code, Name: category.Name, Icon: category.Icon,
		Sort: category.Sort, Status: category.Status,
		CreatedAt: category.CreatedAt, UpdatedAt: category.UpdatedAt,
	}
}

func buildPublicCategoryTree(categories []liveModel.LiveCategory) []liveRes.LiveCategoryPublicItem {
	childrenByParent := make(map[uint][]liveModel.LiveCategory)
	for _, category := range categories {
		childrenByParent[category.ParentId] = append(childrenByParent[category.ParentId], category)
	}
	var build func(uint, map[uint]struct{}) []liveRes.LiveCategoryPublicItem
	build = func(parentID uint, ancestors map[uint]struct{}) []liveRes.LiveCategoryPublicItem {
		items := make([]liveRes.LiveCategoryPublicItem, 0, len(childrenByParent[parentID]))
		for _, category := range childrenByParent[parentID] {
			if _, cycle := ancestors[category.ID]; cycle {
				continue
			}
			nextAncestors := make(map[uint]struct{}, len(ancestors)+1)
			for id := range ancestors {
				nextAncestors[id] = struct{}{}
			}
			nextAncestors[category.ID] = struct{}{}
			items = append(items, liveRes.LiveCategoryPublicItem{
				ID: category.ID, ParentId: category.ParentId, Code: category.Code,
				Name: category.Name, Icon: category.Icon, Sort: category.Sort,
				Children: build(category.ID, nextAncestors),
			})
		}
		return items
	}
	return build(0, map[uint]struct{}{})
}

func buildAdminCategoryTree(categories []liveModel.LiveCategory) []liveRes.LiveCategoryAdminTreeItem {
	childrenByParent := make(map[uint][]liveModel.LiveCategory)
	for _, category := range categories {
		childrenByParent[category.ParentId] = append(childrenByParent[category.ParentId], category)
	}
	var build func(uint, map[uint]struct{}) []liveRes.LiveCategoryAdminTreeItem
	build = func(parentID uint, ancestors map[uint]struct{}) []liveRes.LiveCategoryAdminTreeItem {
		items := make([]liveRes.LiveCategoryAdminTreeItem, 0, len(childrenByParent[parentID]))
		for _, category := range childrenByParent[parentID] {
			if _, cycle := ancestors[category.ID]; cycle {
				continue
			}
			nextAncestors := make(map[uint]struct{}, len(ancestors)+1)
			for id := range ancestors {
				nextAncestors[id] = struct{}{}
			}
			nextAncestors[category.ID] = struct{}{}
			items = append(items, liveRes.LiveCategoryAdminTreeItem{
				ID: category.ID, ParentId: category.ParentId, Code: category.Code,
				Name: category.Name, Icon: category.Icon, Sort: category.Sort, Status: category.Status,
				Children: build(category.ID, nextAncestors),
			})
		}
		return items
	}
	return build(0, map[uint]struct{}{})
}
