package example

import (
	api "tb_live_module/api/v1"
)

type RouterGroup struct {
	CustomerRouter

	AttachmentCategoryRouter
	FileUploadAndDownloadRouter
}

var (
	exaCustomerApi = api.ApiGroupApp.ExampleApiGroup.CustomerApi

	attachmentCategoryApi       = api.ApiGroupApp.ExampleApiGroup.AttachmentCategoryApi
	exaFileUploadAndDownloadApi = api.ApiGroupApp.ExampleApiGroup.FileUploadAndDownloadApi
)
