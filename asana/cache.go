package asana

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

type asection struct {
	list []Basic
}

type acache struct {
	sync.RWMutex
	workspaces  []Basic
	defaultWork string
	projects    []Basic
	tags        []Basic
	users       []Basic
	tagmap      map[string]string
	usermap     map[string]string
	sections    map[string]*asection
}

func printBasics(title string, bs []Basic) {
	return // Avoid unnecessary output. Useful for debugging.

	for _, b := range bs {
		if len(b.Email) > 0 {
			fmt.Printf("%9s %16s %s\n", title, b.Id, b.Email)
		} else {
			fmt.Printf("%9s %16s %s\n", title, b.Id, b.Name)
		}
	}
	fmt.Println()
}

// updateTags updates the tags. Appropriate locks should be acquired by the caller.
func (c *acache) updateTags() error {
	var err error
	c.tags, err = getVarious("tags", "name")
	if err != nil {
		return err
	}
	c.tagmap = make(map[string]string)
	for _, t := range c.tags {
		c.tagmap[t.Id] = t.Name
	}
	printBasics("Tag", c.tags)
	return nil
}

func (c *acache) update() error {
	c.Lock()
	defer c.Unlock()

	var err error
	c.workspaces, err = getVarious("workspaces", "name")
	if err != nil {
		return errors.Wrap(err, "workspaces")
	}
	printBasics("Workspace", c.workspaces)
	for _, w := range c.workspaces {
		if w.Name == *domain {
			c.defaultWork = w.Id
		}
	}
	if c.defaultWork == "" {
		log.Fatalf("Unable to find [%q] domain. Found: %+v", *domain, c.workspaces)
	}

	c.projects, err = getVarious("workspaces/"+c.defaultWork+"/projects", "name")
	if err != nil {
		return errors.Wrap(err, "projects")
	}
	printBasics("Project", c.projects)

	if err := c.updateTags(); err != nil {
		return errors.Wrap(err, "updateTags")
	}

	c.users, err = getVarious("users", "email")
	if err != nil {
		return errors.Wrap(err, "users")
	}
	for i := range c.users {
		u := &c.users[i]
		email := strings.Split(u.Email, "@")
		u.Email = email[0]
	}
	c.usermap = make(map[string]string)
	for _, u := range c.users {
		c.usermap[u.Id] = u.Email
	}
	printBasics("User", c.users)
	c.sections = make(map[string]*asection)
	return nil
}

func (c *acache) Workspace() string {
	c.RLock()
	defer c.RUnlock()
	return c.defaultWork
}

func (c *acache) Projects() []Basic {
	c.RLock()
	defer c.RUnlock()
	projects := make([]Basic, len(c.projects))
	copy(projects, c.projects)
	return projects
}

func (c *acache) ProjectId(name string) string {
	c.RLock()
	defer c.RUnlock()
	for _, p := range c.projects {
		if p.Name == name {
			return p.Id
		}
	}
	return ""
}

func (c *acache) User(uid string) string {
	c.RLock()
	defer c.RUnlock()
	return c.usermap[uid]
}

func (c *acache) UserId(email string) string {
	c.RLock()
	defer c.RUnlock()
	for _, u := range c.users {
		if email == u.Email {
			return u.Id
		}
	}
	return ""
}

func (c *acache) Tag(uid string) string {
	c.RLock()
	defer c.RUnlock()
	return c.tagmap[uid]
}

func (c *acache) TagId(tname string) string {
	c.RLock()
	c.RUnlock()
	for _, t := range c.tags {
		if t.Name == tname {
			return t.Id
		}
	}
	return ""
}

func (c *acache) CreateTag(tname string) string {
	c.Lock()
	defer c.Unlock()

	// Just double check after acquiring lock.
	for _, t := range c.tags {
		if t.Name == tname {
			return t.Id
		}
	}

	v := url.Values{}
	v.Add("workspace", c.defaultWork)
	v.Add("name", tname)
	resp, err := runPost("POST", "tags", v)
	if err != nil {
		return ""
	}
	var bdo BasicDataOne
	if err := json.Unmarshal(resp, &bdo); err != nil {
		return ""
	}
	c.tags = append(c.tags, bdo.Data)
	c.tagmap[bdo.Data.Id] = bdo.Data.Name

	return bdo.Data.Id
}

func (c *acache) AddSection(projId string, sec Basic) string {
	c.Lock()
	defer c.Unlock()
	s, found := c.sections[projId]
	if !found {
		s = new(asection)
		c.sections[projId] = s
	}
	if !strings.HasSuffix(sec.Name, ":") {
		return ""
	}

	sec.Name = strings.Map(func(r rune) rune {
		if 'A' <= r && r <= 'Z' || 'a' <= r && r <= 'z' || '0' <= r && r <= '9' {
			return r
		}
		return -1
	}, sec.Name)

	for i := range s.list {
		l := &s.list[i]
		if l.Id == sec.Id {
			l.Name = sec.Name
			return sec.Name
		}
	}
	s.list = append(s.list, sec)
	return sec.Name
}

func (c *acache) SectionName(projId string, secId string) string {
	c.RLock()
	defer c.RUnlock()
	s, found := c.sections[projId]
	if !found {
		return ""
	}
	for _, l := range s.list {
		if l.Id == secId {
			return l.Name
		}
	}
	return ""
}

func (c *acache) SectionId(projId string, sectionName string) string {
	c.RLock()
	defer c.RUnlock()
	s, found := c.sections[projId]
	if !found {
		return ""
	}
	for _, l := range s.list {
		if l.Name == sectionName {
			return l.Id
		}
	}
	return ""
}
