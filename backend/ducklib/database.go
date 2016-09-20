package ducklib

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"

	"github.com/Microsoft/DUCK/backend/ducklib/structs"
	"github.com/Microsoft/DUCK/backend/pluginregistry"
	"github.com/twinj/uuid"
)

type database struct {
	Config structs.DBConf
}

var db pluginregistry.DBPlugin

//NewDatabase returns an intialized database struct
func NewDatabase(config structs.DBConf) *database {
	return &database{Config: config}
}

//Put this into plugin
func FillTestdata(testDataFile string) error {

	var listOfData []interface{}

	dat, err := ioutil.ReadFile(testDataFile)
	if err != nil {
		log.Printf("Error in FillTestdata while trying to read from the file: %s", err)
		return err
	}

	if err := json.Unmarshal(dat, &listOfData); err != nil {
		return err
	}
	for _, l := range listOfData {

		mp := l.(map[string]interface{})

		entryType := mp["type"].(string)

		switch entryType {
		case "document":
			var d structs.Document
			d.FromValueMap(mp)

			if err := db.NewDocument(d); err != nil {
				return err
			}
		case "user":
			var u structs.User
			u.FromValueMap(mp)
			if err := db.NewUser(u); err != nil {
				return err
			}

		}

	}

	return nil
}

//Init initializes the database and checks for connection errors
func (database *database) Init() error {

	db = pluginregistry.DatabasePlugin

	err := db.Init(database.Config)
	if err != nil {
		return err
	}
	return nil
}

/*
User DB operations
*/

//GetLogin returns id and password for username
func (database *database) GetLogin(email string) (id string, pw string, err error) {
	return db.GetLogin(email)
}

func (database *database) GetUser(userid string) (structs.User, error) {

	return db.GetUser(userid)

}

func (database *database) DeleteUser(id string) error {

	return db.DeleteUser(id)

}

func (database *database) PutUser(user structs.User) error {

	return db.UpdateUser(user)

}

func (database *database) PostUser(user structs.User) (ID string, err error) {
	//check for duplicate
	if user.Email == "" {
		return "", errors.New("No email submitted")
	}
	if user.Password == "" {
		return "", errors.New("No password submitted")
	}

	_, _, err = db.GetLogin(user.Email)

	// if user is not in dtabase we can create a new one
	//TODO: What if another Database Plugin returns another Error when getting an nonexistant User?
	if err != nil && err.Error() == "User not found" {
		u := uuid.NewV4()
		uuid := uuid.Formatter(u, uuid.Clean)
		user.ID = uuid
		return uuid, db.NewUser(user)

	}

	//We don't know if the user exists because we got an error checking this
	if err != nil {
		return "", err
	}

	return "", errors.New("User already exists")
}

func (database *database) GetUserDict(userid string) (structs.Dictionary, error) {

	return db.GetUserDict(userid)

}
func (database *database) PutUserDict(dict structs.Dictionary, userID string) error {

	return db.UpdateUserDict(dict, userID)

}

/*
Document DB operations

*/
func (database *database) GetDocument(documentid string) (structs.Document, error) {

	return db.GetDocument(documentid)

}
func (database *database) GetDocumentSummariesForUser(userid string) ([]structs.Document, error) {

	return db.GetDocumentSummariesForUser(userid)

}

func (database *database) DeleteDocument(id string) error {

	return db.DeleteDocument(id)

}

func (database *database) PutDocument(doc structs.Document) error {

	return db.UpdateDocument(doc)

}

func (database *database) PostDocument(doc structs.Document) (ID string, err error) {
	if doc.Name == "" {
		return "", errors.New("No Document Name submitted")
	}

	if doc.Owner == "" {
		return "", errors.New("No Document Owner submitted")
	}

	smap := make(map[string]bool)
	for _, s := range doc.Statements {
		if _, prs := smap[s.TrackingID]; prs {
			return "", errors.New("Document contains two Statements with the same statement ID")
		}
		smap[s.TrackingID] = true
	}

	u := uuid.NewV4()
	uuid := uuid.Formatter(u, uuid.Clean)
	doc.ID = uuid

	
	return uuid, db.NewDocument(doc)

}

/*
Rulebase DB operations
*/
/*
func (database *Database) GetRulebase(id string) (User, error) {
	var u User
	mp, err := db.GetRulebase(id)
	if err != nil {
		return u, err
	}

	u.fromValueMap(mp)

	return u, err
}

func (database *Database) DeleteRulebase(id string) error {

	doc, err := db.GetRulebase(id)
	if err != nil {

		return err
	}
	if rev, prs := doc["_rev"]; prs {
		err := db.DeleteRulebase(id, rev.(string))
		if err != nil {
			return err
		}
		return nil
	}

	return errors.New("Could not delete Entry")

}

func (database *Database) PutRulebase(id string, content []byte) error {

	return db.UpdateRulebase(id, string(content))

}

func (database *Database) PostRulebase(content []byte) (string, error) {
	u := uuid.NewV4()
	uuid := uuid.Formatter(u, uuid.Clean)

	return uuid, db.NewRulebase(uuid, string(content))

}
*/
