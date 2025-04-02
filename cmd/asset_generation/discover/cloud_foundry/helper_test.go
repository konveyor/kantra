package cloud_foundry

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Cloud Foundry Functions", func() {

	Describe("NormalizeForMetadataName", func() {
		It("should return an error for an empty string", func() {
			_, err := cloud_foundry.NormalizeForMetadataName("")
			Expect(err).Should(HaveOccurred())
			Expect(err.Error()).To(MatchError("failed to normalize for service/metadata name because it is an empty string"))
		})

		It("should convert to lowercase", func() {
			normalized, err := cloud_foundry.NormalizeForMetadataName("UPPERCASE")
			Expect(err).NotTo(HaveOccurred())
			Expect(normalized).To(Equal("uppercase"))
		})

		It("should replace disallowed characters with hyphens", func() {
			normalized, err := cloud_foundry.NormalizeForMetadataName("invalid_name")
			Expect(err).NotTo(HaveOccurred())
			Expect(normalized).To(Equal("invalid-name"))
		})

		It("should truncate names longer than 63 characters", func() {
			longName := "this-is-a-very-long-name-that-exceeds-the-maximum-length-allowed"
			normalized, err := cloud_foundry.NormalizeForMetadataName(longName)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(normalized)).To(Equal(63))
		})

		It("should replace starting hyphens with 'a'", func() {
			normalized, err := cloud_foundry.NormalizeForMetadataName("-name")
			Expect(err).NotTo(HaveOccurred())
			Expect(normalized).To(Equal("aname"))
		})

		It("should replace terminating hyphens with 'z'", func() {
			normalized, err := cloud_foundry.NormalizeForMetadataName("name-")
			Expect(err).NotTo(HaveOccurred())
			Expect(normalized).To(Equal("namez"))
		})
	})