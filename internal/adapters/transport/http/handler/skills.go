package handler

import (
	appskill "fkteams/internal/app/skill"
	"fkteams/internal/runtime/log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GetInstalledSkillsHandler 返回已安装技能列表。
func GetInstalledSkillsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		skills, err := appskill.ListLocalSkills()
		if err != nil {
			log.Printf("failed to list skills: %v", err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if skills == nil {
			skills = []appskill.LocalSkillInfo{}
		}
		OK(c, gin.H{"skills": skills, "total": len(skills)})
	}
}

// SearchSkillsHandler 搜索技能市场。
func SearchSkillsHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		keyword := c.Query("q")
		if keyword == "" {
			Fail(c, http.StatusBadRequest, "keyword is required")
			return
		}

		page := 1
		if p := c.Query("page"); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n > 0 {
				page = n
			}
		}
		size := 20
		if s := c.Query("size"); s != "" {
			if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 50 {
				size = n
			}
		}
		sortBy := c.Query("sort")
		if sortBy == "" {
			sortBy = "downloads"
		}
		order := c.Query("order")
		if order == "" {
			order = "desc"
		}

		provider := appskill.GetDefaultProvider()
		if provider == nil {
			Fail(c, http.StatusServiceUnavailable, "no skill provider available")
			return
		}

		resp, err := provider.Search(c.Request.Context(), keyword, page, size, sortBy, order)
		if err != nil {
			log.Printf("failed to search skills: %v", err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}
		if resp == nil {
			OK(c, gin.H{"skills": []appskill.SkillResult{}, "total": 0})
			return
		}

		OK(c, gin.H{"skills": resp.Skills, "total": resp.Total, "page": page, "size": size})
	}
}

// InstallSkillHandler 从技能市场安装技能。
func InstallSkillHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Slug string `json:"slug"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Slug == "" {
			Fail(c, http.StatusBadRequest, "slug is required")
			return
		}

		provider := appskill.GetDefaultProvider()
		if provider == nil {
			Fail(c, http.StatusServiceUnavailable, "no skill provider available")
			return
		}

		if err := appskill.InstallSkillFromProvider(c.Request.Context(), req.Slug, "", provider); err != nil {
			log.Printf("failed to install skill: slug=%s, err=%v", req.Slug, err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		OK(c, gin.H{"slug": req.Slug, "message": "skill installed"})
	}
}

// RemoveSkillHandler 删除已安装技能。
func RemoveSkillHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if slug == "" {
			Fail(c, http.StatusBadRequest, "slug is required")
			return
		}

		if err := appskill.RemoveLocalSkill(slug); err != nil {
			log.Printf("failed to remove skill: slug=%s, err=%v", slug, err)
			Fail(c, http.StatusInternalServerError, err.Error())
			return
		}

		OK(c, gin.H{"slug": slug, "message": "skill removed"})
	}
}

// GetSkillFilesHandler 返回技能文件树。
func GetSkillFilesHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		if slug == "" {
			Fail(c, http.StatusBadRequest, "slug is required")
			return
		}

		files, err := appskill.ListSkillFiles(slug, c.Query("path"))
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}
		if files == nil {
			files = []appskill.SkillFileEntry{}
		}

		OK(c, gin.H{"slug": slug, "files": files})
	}
}

// GetSkillFileContentHandler 读取技能文件内容。
func GetSkillFileContentHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		slug := c.Param("slug")
		filePath := c.Query("path")
		if slug == "" || filePath == "" {
			Fail(c, http.StatusBadRequest, "slug and path are required")
			return
		}

		content, err := appskill.ReadSkillFile(slug, filePath)
		if err != nil {
			Fail(c, http.StatusNotFound, err.Error())
			return
		}

		OK(c, gin.H{"slug": slug, "path": filePath, "content": content})
	}
}
