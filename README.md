# Forum for Alem Community


## How to run
```
go run .
```

User features:

- [x] Registration with encrypted store of passwords in DB
- [ ] Email Verification
- [x] Login with JWT issue & logout with JWT removal
- [ ] Login/Registration using GitHub
- [x] Get posts details [author, title, text, timestamp, categories, likes, dislikes]:
- [x] - by category
- [x] - by status
- [x] - by user id
- [x] - by post id
- [x] - by search pattern
- [x] - by most liked/disliked posts
- [x] - with pagination
- [x] Write/update/delete post
- [x] Write/update/delete comment
- [x] Like/dislike post or comment
- [x] Post or comment report
- [x] Avatar upload
- [x] Post image upload (return as new url)
- [ ] Single user profile view (likes, dislikes, posts, comments)

Websocket features:

- [ ] Posts live-apperance
- [ ] Comments live-apperance
- [ ] Likes/Dislikes count live-changing
- [ ] Who's online
- [ ] Notifications

Admin features:

- [x] Category creation & edit
- [x] Category delete
- [x] List all users
- [x] Changing user's role

Admin & Moderator features:

- [x] User's post and comment edit
- [x] Reports review
- [x] Report status change
- [ ] User suspension with due time
