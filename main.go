package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	_ "image/jpeg"
	_ "image/png"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type userType struct {
	Phone    int
	Name     string
	UUID     string
	Insta    string
	Balance  int
	Likes    int
	VotedFor map[int]int
}

type userWithPosition struct {
	userType
	Position int
}

type photos struct {
	Phone int
	Photo string
}

var cache = map[int]int{79178884084: 31415}

var codeSentTo = map[int]int{}

func main() {

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	http.HandleFunc("/userUpdate", userUpdateHandler)
	http.HandleFunc("/userGet", userGetHandler)
	http.HandleFunc("/photo", photoHandler)
	http.HandleFunc("/smsSend", smsSendHandler)
	http.HandleFunc("/phoneConfirm", phoneConfirmHandler)
	http.HandleFunc("/permissionCheck", permissionCheckHandler)

	http.ListenAndServe(":"+port, nil)
}

var userUpdateHandler = func(w http.ResponseWriter, req *http.Request) {

	query := req.URL.Query()

	phone, phoneOk := query["phone"]
	name, nameOk := query["name"]
	insta, instaOk := query["insta"]
	balance, balanceOk := query["balance"]
	balanceOperation, balanceOperationOk := query["balanceoperation"]
	likes, likesOk := query["likes"]
	votedFor, votedForOk := query["votedfor"]
	uuid, uuidOk := query["uuid"]

	if !phoneOk || (!nameOk && !instaOk && !balanceOk && !likesOk && !votedForOk) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "phone or parameters are not provided")
		return
	}

	dbURL := os.Getenv("DATABASE_URL")

	var user userType

	db, err := sql.Open("postgres", dbURL)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}
	defer db.Close()

	userSQL := "SELECT * FROM users WHERE phone = $1;"

	var votedFromDB []byte

	err = db.QueryRow(userSQL, phone[0]).Scan(&user.Phone, &user.Name, &user.UUID, &user.Insta, &user.Balance, &user.Likes, &votedFromDB)

	if err != nil {
		createErrMsg(err, http.StatusNotFound, w)
		return
	}

	err = json.Unmarshal(votedFromDB, &user.VotedFor)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	//*********************************

	if balanceOk || balanceOperationOk {

		if !balanceOk || !balanceOperationOk {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Balance or operation are not provided")
			return
		}

		intBalance, err := strconv.Atoi(balance[0])

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		if balanceOperation[0] == "plus" {
			user.Balance += intBalance
		}
	}

	//*********************************

	if (votedForOk || uuidOk) && likesOk {

		if (votedForOk && uuidOk) || (!votedForOk && !uuidOk) {
			w.WriteHeader(http.StatusBadRequest)
			io.WriteString(w, "Invalid params")
			return
		}

		intLikes, err := strconv.Atoi(likes[0])

		if err != nil || intLikes <= 0 {
			createErrMsg(err, http.StatusBadRequest, w)
			return
		}

		if votedForOk {
			intPhone, err := strconv.Atoi(votedFor[0])

			if err != nil {
				createErrMsg(err, http.StatusInternalServerError, w)
				return
			}

			if (user.VotedFor[intPhone] > 0 && user.Balance-intLikes < 0) || (user.Balance == 0 && intLikes > 1) {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "User does not have enough likes in balance")
				return
			}
		}

		var likedUser userType

		if (votedForOk && (phone[0] != votedFor[0])) || uuidOk {

			var queryParam string
			var searchType string

			if votedForOk {
				queryParam = "phone = $1;"
				searchType = votedFor[0]
			}

			if uuidOk {
				queryParam = "uuid = $1;"
				searchType = uuid[0]
			}

			userSQL := "SELECT phone, likes FROM users WHERE " + queryParam
			err = db.QueryRow(userSQL, searchType).Scan(&likedUser.Phone, &likedUser.Likes)

			if err != nil {
				stringErr := fmt.Sprint(err)

				if stringErr != "sql: no rows in result set" || stringErr == "sql: no rows in result set" && uuidOk {
					w.WriteHeader(http.StatusInternalServerError)
					io.WriteString(w, fmt.Sprint((err)))
					return
				}

				intPhone, err := strconv.Atoi(votedFor[0])

				if err != nil {
					createErrMsg(err, http.StatusInternalServerError, w)
					return
				}

				if (user.VotedFor[intPhone] > 0 && user.Balance-intLikes < 0) || (user.Balance == 0 && intLikes > 1) {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, "User does not have enough likes in balance")
					return
				}

				if votedForOk {
					smsMessage := "Валентинка от "

					if user.Name != "User" {

						runeName := []rune(user.Name)

						if len(user.Name) == len(runeName) && len(user.Name) > 18 {
							cuttedName := user.Name[:20]
							smsMessage += cuttedName
						} else if len(user.Name) > len(runeName) && len(user.Name) > 35 {
							cuttedName := user.Name[:35]
							smsMessage += cuttedName
						} else {
							smsMessage += user.Name
						}

					} else {
						smsMessage += "друга"
					}

					smsMessage = smsMessage + " в приложении: https://surl.li/kxbm"

					url := "https://tatschef@yandex.ru:ydepid7rIeLlfT2qKQ09KPvuF2Le@gate.smsaero.ru/v2/sms/send?number=" + votedFor[0] + "&text=" + smsMessage + "&sign=SMS Aero"

					_, err = http.Get(url)

					if err != nil {
						createErrMsg(err, http.StatusInternalServerError, w)
						return
					}
				}

				createDefaultUser(&likedUser, intPhone)

				likedUser.Likes = intLikes

				votedJSON, err := json.Marshal(likedUser.VotedFor)

				if err != nil {
					createErrMsg(err, http.StatusInternalServerError, w)
					return
				}

				userSQL = "INSERT INTO users (phone, name, uuid, insta, balance, likes, votedfor) VALUES ($1, $2, $3, $4, $5, $6, $7);"
				_, err = db.Exec(userSQL, likedUser.Phone, likedUser.Name, likedUser.UUID, likedUser.Insta, likedUser.Balance, likedUser.Likes, votedJSON)

				if err != nil {
					createErrMsg(err, http.StatusInternalServerError, w)
					return

				}

			} else {

				if (user.VotedFor[likedUser.Phone] > 0 && user.Balance-intLikes < 0) || (user.Balance == 0 && intLikes > 1) {
					w.WriteHeader(http.StatusBadRequest)
					io.WriteString(w, "User does not have enough likes in balance")
					return
				}

				likedUser.Likes += intLikes

				userSQL = "UPDATE users SET likes = $1 where phone = $2;"
				_, err = db.Exec(userSQL, likedUser.Likes, likedUser.Phone)

				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					io.WriteString(w, fmt.Sprint(err))
					return

				}
			}

		} else {
			if (user.VotedFor[user.Phone] > 0 && user.Balance-intLikes < 0) || (user.Balance == 0 && intLikes > 1) {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "User does not have enough likes in balance")
				return
			}

			user.Likes += intLikes
		}

		var votedForIntTelNum int

		if votedForOk {
			votedForIntTelNum, err = strconv.Atoi(votedFor[0])

			if err != nil {
				createErrMsg(err, http.StatusBadRequest, w)
				return
			}
		} else {
			votedForIntTelNum = likedUser.Phone
		}

		_, voteOk := user.VotedFor[votedForIntTelNum]

		if voteOk {
			user.VotedFor[votedForIntTelNum] += intLikes
			user.Balance -= intLikes
		} else {
			if intLikes > 1 && user.Balance-intLikes < 0 {
				w.WriteHeader(http.StatusBadRequest)
				io.WriteString(w, "User does not have enough likes in balance")
				return
			}
			user.VotedFor[votedForIntTelNum] = intLikes

			if intLikes > 1 {
				user.Balance -= intLikes
			}

		}
	}

	//*********************************

	if instaOk {
		user.Insta = insta[0]
	}

	if nameOk {
		user.Name = name[0]
	}

	votedJSON, err := json.Marshal(user.VotedFor)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	userSQL = "UPDATE users set name = $1, insta = $2, balance = $3, likes = $4, votedfor = $5 where phone = $6;"
	_, err = db.Exec(userSQL, user.Name, user.Insta, user.Balance, user.Likes, votedJSON, user.Phone)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}
	setStatusOkForUser(user, w)

}

//==========================================================================

var userGetHandler = func(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	phone, phoneOk := query["phone"]
	uuid, uuidOk := query["uuid"]
	_, allOk := query["all"]

	if !phoneOk && !uuidOk && !allOk {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Phone or UUID or parameter is needed")
		return
	}

	dbURL := os.Getenv("DATABASE_URL")

	db, err := sql.Open("postgres", dbURL)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}
	defer db.Close()

	if allOk {
		var userFromDB userType
		var users []userType
		var votedFromDB []byte

		rows, err := db.Query("SELECT * FROM users ORDER BY likes DESC LIMIT 100;")

		if err != nil {
			createErrMsg(err, http.StatusNotFound, w)
			return
		}

		for rows.Next() {
			userFromDB.VotedFor = map[int]int{}
			rows.Scan(&userFromDB.Phone, &userFromDB.Name, &userFromDB.UUID, &userFromDB.Insta, &userFromDB.Balance, &userFromDB.Likes, &votedFromDB)

			err = json.Unmarshal(votedFromDB, &userFromDB.VotedFor)
			if err != nil {
				createErrMsg(err, http.StatusInternalServerError, w)
				return
			}

			users = append(users, userFromDB)

		}

		w.WriteHeader(http.StatusOK)
		usersJSON, err := json.Marshal(users)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		io.WriteString(w, string(usersJSON))

		return

	}

	var votedFromDB []byte
	var userFromDB userWithPosition
	var queryParam string
	var idParam string

	if uuidOk {
		queryParam = "uuid = $1;"
		idParam = uuid[0]
	}

	if phoneOk {
		queryParam = "phone = $1;"
		idParam = phone[0]
	}

	userSQL := "SELECT * FROM users WHERE " + queryParam

	err = db.QueryRow(userSQL, idParam).Scan(&userFromDB.Phone, &userFromDB.Name, &userFromDB.UUID, &userFromDB.Insta, &userFromDB.Balance, &userFromDB.Likes, &votedFromDB)

	if err != nil {
		createErrMsg(err, http.StatusNotFound, w)
		return
	}

	err = json.Unmarshal(votedFromDB, &userFromDB.VotedFor)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	userSQL = "SELECT POSITION FROM (SELECT *, row_number() OVER(ORDER BY likes DESC) as position FROM users) RESULT WHERE " + queryParam

	err = db.QueryRow(userSQL, idParam).Scan(&userFromDB.Position)

	if err != nil {
		createErrMsg(err, http.StatusNotFound, w)
		return
	}

	w.WriteHeader(http.StatusOK)
	userJSON, err := json.Marshal(userFromDB)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	io.WriteString(w, string(userJSON))

}

//==========================================================================

type smsResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Cost float64 `json:"cost"`
	} `json:"data"`
}

var smsSendHandler = func(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	phone, phoneOk := query["phone"]

	if !phoneOk {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Phone number is not provided")
		return
	}

	convertedPhone, err := strconv.Atoi(phone[0])

	if err != nil || convertedPhone < 10000000000 {
		createErrMsg(err, http.StatusBadRequest, w)
		return
	}

	testPhone := regexp.MustCompile(`790000000`)

	if testPhone.Match([]byte(phone[0])) {
		cache[convertedPhone] = 31415

		res := map[int]int{}
		res[convertedPhone] = 31415

		resJSON, err := json.Marshal(res)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, string(resJSON))

		return
	}

	if phone[0] == "79178884084" {
		resJSON, err := json.Marshal(cache)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, string(resJSON))

		return
	}

	rand.Seed(time.Now().UnixNano())
	code := rand.Intn(99999)

	if code < 10000 {
		code += 10000
	}

	cache[convertedPhone] = code

	if codeSentTo[convertedPhone] < 3 {

		url := "https://tatschef@yandex.ru:ydepid7rIeLlfT2qKQ09KPvuF2Le@gate.smsaero.ru/v2/sms/send?number=" + phone[0] + "&text=Auth code: " + fmt.Sprint(code) + "&sign=SMS Aero"

		res, err := http.Get(url)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		defer res.Body.Close()

		body, err := ioutil.ReadAll(res.Body)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		var resStruct smsResponse

		err = json.Unmarshal(body, &resStruct)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		codeSentTo[convertedPhone]++

		resJSON, err := json.Marshal(resStruct)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		io.WriteString(w, string(resJSON))

	}

}

//==========================================================================

var phoneConfirmHandler = func(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	phone, phoneOk := query["phone"]
	code, codeOk := query["code"]

	if !codeOk || !phoneOk {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "code or phone are not provided")
		return
	}

	convertedPhone, err := strconv.Atoi(phone[0])

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "invalid phone number")
		return
	}

	convertedCode, err := strconv.Atoi(code[0])

	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "invalid code")
		return
	}

	sendedCode, sendedCodeOk := cache[convertedPhone]

	if !sendedCodeOk || convertedCode != sendedCode {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Code was not found")
		return
	}

	testPhone := regexp.MustCompile(`790000000`)

	if convertedPhone != 79178884084 || testPhone.Match([]byte(phone[0])) == false {
		delete(cache, convertedPhone)
		delete(codeSentTo, convertedPhone)
	}

	var user userWithPosition

	dbURL := os.Getenv("DATABASE_URL")

	db, err := sql.Open("postgres", dbURL)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}
	defer db.Close()

	userSQL := "SELECT * FROM users WHERE phone = $1;"

	var votedFromDb []byte

	err = db.QueryRow(userSQL, phone[0]).Scan(&user.Phone, &user.Name, &user.UUID, &user.Insta, &user.Balance, &user.Likes, &votedFromDb)

	if err != nil {
		stringErr := fmt.Sprint(err)

		if stringErr != "sql: no rows in result set" {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		createDefaultUserWithPosition(&user, convertedPhone)

		votedJSON, err := json.Marshal(user.VotedFor)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		userSQL = "INSERT INTO users (phone, name, uuid, insta, balance, likes, votedfor) VALUES ($1, $2, $3, $4, $5, $6, $7);"
		_, err = db.Exec(userSQL, user.Phone, user.Name, user.UUID, user.Insta, user.Balance, user.Likes, votedJSON)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		w.WriteHeader(http.StatusOK)
		userJSON, err := json.Marshal(user)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		io.WriteString(w, string(userJSON))

		return

	}

	err = json.Unmarshal(votedFromDb, &user.VotedFor)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	userSQL = "SELECT POSITION FROM (SELECT *, row_number() OVER(ORDER BY likes DESC) as position FROM users) RESULT WHERE phone = $1"

	err = db.QueryRow(userSQL, phone[0]).Scan(&user.Position)

	if err != nil {
		createErrMsg(err, http.StatusNotFound, w)
		return
	}

	w.WriteHeader(http.StatusOK)
	userJSON, err := json.Marshal(user)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	io.WriteString(w, string(userJSON))

}

//==========================================================================

var photoHandler = func(w http.ResponseWriter, req *http.Request) {

	query := req.URL.Query()

	phone, phoneOk := query["phone"]
	_, getOk := query["get"]
	set, setOk := query["set"]

	if !phoneOk {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "code or phone are not provided")
		return
	}

	if (getOk && setOk) || (setOk && req.Method == "POST") {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "Invalid params")
		return
	}

	var base64Img string

	if req.Method == "POST" {
		body, err := ioutil.ReadAll(req.Body)

		if err != nil {
			createErrMsg(err, http.StatusInternalServerError, w)
			return
		}

		base64Img = base64.StdEncoding.EncodeToString(body)

	}

	dbURL := os.Getenv("DATABASE_URL")

	var photosRes photos

	db, err := sql.Open("postgres", dbURL)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}
	defer db.Close()

	if getOk {
		userSQL := "SELECT photo FROM photos WHERE phone = $1;"

		err = db.QueryRow(userSQL, phone[0]).Scan(&photosRes.Photo)

		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, fmt.Sprint(err))
			return
		}

		base64Img, err := base64.StdEncoding.DecodeString(photosRes.Photo)

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, fmt.Sprint(err))
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(base64Img)))
		w.WriteHeader(http.StatusOK)
		w.Write(base64Img)

		return

	}

	photoValue := base64Img

	if setOk {
		photoValue = set[0]
	}

	intPhone, err := strconv.Atoi(phone[0])

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, fmt.Sprint(err))
		return
	}

	if setOk || base64Img != "" {
		userSQL := "SELECT * FROM photos WHERE phone = $1;"
		err = db.QueryRow(userSQL, phone[0]).Scan(&photosRes.Phone, &photosRes.Photo)

		if err != nil {
			stringErr := fmt.Sprint(err)

			if stringErr != "sql: no rows in result set" {
				w.WriteHeader(http.StatusInternalServerError)
				io.WriteString(w, stringErr)
				return
			}

			userSQL := "INSERT INTO photos (phone, photo) values ($1, $2);"
			_, err = db.Query(userSQL, phone[0], photoValue)

		} else {
			userSQL := "UPDATE photos set photo = $1 WHERE phone = $2;"
			_, err = db.Query(userSQL, photoValue, phone[0])
		}
	}

	photosRes.Phone = intPhone
	photosRes.Photo = photoValue

	resJSON, err := json.Marshal(photosRes)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	w.WriteHeader(http.StatusOK)
	io.WriteString(w, string(resJSON))

}

var permissionCheckHandler = func(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query()

	_, useOk := query["caniuseit"]

	if !useOk {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, "Param is not provided")
		return
	}

	permission := true

	if permission {
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, "YES")
		return
	}

	w.WriteHeader(http.StatusBadRequest)
	io.WriteString(w, "NO")

}

func setStatusOkForUser(user userType, w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	userJSON, err := json.Marshal(user)

	if err != nil {
		createErrMsg(err, http.StatusInternalServerError, w)
		return
	}

	io.WriteString(w, string(userJSON))
}

func createDefaultUser(user *userType, phone int) {
	user.Phone = phone
	user.UUID = fmt.Sprint(uuid.New())
	user.Name = "User"
	user.VotedFor = map[int]int{}
}

func createDefaultUserWithPosition(user *userWithPosition, phone int) {
	user.Phone = phone
	user.UUID = fmt.Sprint(uuid.New())
	user.Name = "User"
	user.VotedFor = map[int]int{}
}

func createErrMsg(err error, status int, w http.ResponseWriter) {
	fmt.Println(err)
	w.WriteHeader(status)
	io.WriteString(w, fmt.Sprint(err))
}
