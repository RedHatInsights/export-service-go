package s3_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redhatinsights/export-service-go/s3"
)

var _ = Describe("Build files included in zip", func() {
	It("should build the meta.json file provided ExportMeta struct", func() {
		// Make the ExportMeta struct
		meta := s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters: map[string]string{
						"filter_key": "filter_value",
					},
				},
			},
			HelpString: "Help me!",
			FailedFiles: []s3.FailedFileMeta{
				{
					Filename: 	 "filename",
					Application: "application",
					Resource: 	 "resource",
					Error: 		s3.ExportError{"code","message"},
				},
			},
		}

		metaDump, err := s3.BuildMeta(&meta)

		Expect(err).To(BeNil())
		Expect(metaDump).To(Equal([]byte(`{"exported_by":"user","export_date":"date","export_org_id":"org_id","file_meta":[{"filename":"filename","application":"application","resource":"resource","filters":{"filter_key":"filter_value"}}],"help_string":"Help me!","failed_files":[{"filename":"filename","application":"application","resource":"resource","error":{"code":"code","message":"message"}}]}`)))

		// Error should never occur, not even for nil case
		_, err = s3.BuildMeta(nil)
		Expect(err).To(BeNil())
	})

	DescribeTable("Test the BuildReadme function", func(meta s3.ExportMeta, expected string) {
		readme, err := s3.BuildReadme(&meta)
		Expect(err).To(BeNil())

		Expect(readme).To(Equal(expected))
	},
		Entry("an empty ExportMeta struct", s3.ExportMeta{},
			`# Export Manifest

## Exported Information
- **Exported by**: 
- **Org ID**: 
- **Export Date**: 

## Data Details
This archive contains the following data:

No data was found.


## Failed Files

No data was found!


## Help and Support
This service is owned by the ConsoleDot Pipeline team. If you have any questions, or need support with this service, please contact Red Hat Support.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("No sources", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			HelpString:  "Help me!",
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

No data was found.


## Failed Files

No data was found!


## Help and Support
This service is owned by the ConsoleDot Pipeline team. If you have any questions, or need support with this service, please contact Red Hat Support.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("One source with filters", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters:     map[string]string{},
				},
			},
			HelpString: "Help me!",
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

### filename
- **Application**: application
- **Resource**: resource
- **Filters**: None


## Failed Files

No data was found!


## Help and Support
This service is owned by the ConsoleDot Pipeline team. If you have any questions, or need support with this service, please contact Red Hat Support.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
		Entry("Multiple sources", s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters: map[string]string{
						"filter_key": "filter_value",
					},
				},
				{
					Filename:    "filename2",
					Application: "application2",
					Resource:    "resource2",
					Filters: map[string]string{
						"filter_key": "filter_value",
					},
				},
			},
			HelpString: "Help me!",
			FailedFiles: []s3.FailedFileMeta{
				{
					Filename: 	 "filename",
					Application: "application",
					Resource: 	 "resource",
					Error: 		s3.ExportError{"code","message"},
				},
			},
		},
			`# Export Manifest

## Exported Information
- **Exported by**: user
- **Org ID**: org_id
- **Export Date**: date

## Data Details
This archive contains the following data:

### filename
- **Application**: application
- **Resource**: resource
- **Filters**: 
  - filter_key: filter_value

### filename2
- **Application**: application2
- **Resource**: resource2
- **Filters**: 
  - filter_key: filter_value


## Failed Files

### filename
- **Application**: application
- **Resource**: resource
- **Error Code**: code
- **Error Message**: message


## Help and Support
This service is owned by the ConsoleDot Pipeline team. If you have any questions, or need support with this service, please contact Red Hat Support.

You can also raise an issue on the [Export Service GitHub repo](https://github.com/RedHatInsights/export-service-go/).
`,
		),
	)

	It("test the filters are right", func() {
		filters := map[string]string{
			"filter_key":  "filter_value",
			"filter_key2": "filter_value2",
			"filter_key3": "filter_value3",
			"filter_key4": "filter_value4",
			"filter_key5": "filter_value5",
		}
		// Make the ExportMeta struct
		meta := s3.ExportMeta{
			ExportBy:    "user",
			ExportDate:  "date",
			ExportOrgID: "org_id",
			FileMeta: []s3.ExportFileMeta{
				{
					Filename:    "filename",
					Application: "application",
					Resource:    "resource",
					Filters:     filters,
				},
			},
			HelpString: "Help me!",
			FailedFiles: []s3.FailedFileMeta{
				{
					Filename: 	 "filename",
					Application: "application",
					Resource: 	 "resource",
					Error: 		s3.ExportError{"code","message"},
				},
			},
		}

		metaDump, err := s3.BuildMeta(&meta)

		Expect(err).To(BeNil())

		// Make sure each of the filters are in the metaDump
		for key, value := range filters {
			Expect(string(metaDump)).To(ContainSubstring(fmt.Sprintf("\"%s\":\"%s\"", key, value)))
		}

		// get the readme
		readme, err := s3.BuildReadme(&meta)
		Expect(err).To(BeNil())

		// Make sure each of the filters are in the readme
		for key, value := range filters {
			Expect(string(readme)).To(ContainSubstring(fmt.Sprintf("%s: %s", key, value)))
		}
	})
})
