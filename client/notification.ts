// Desktop notifications on reply and such

import { storeSeenReply, isMobile } from "./state"
import options from "./options"
import { posts as lang } from "./lang"
import { thumbPath } from "./posts/render/image"
import { repliedToMe } from "./tab"
import { Post } from "./posts/models"
import { scrollToAnchor } from "./scroll"
import { read, write } from "./render"

// Displayed, when there is no image in post
const defaultIcon = "/assets/notification-icon.png"

// Notify the user that one of their posts has been replied to
export default function notifyAboutReply(post: Post) {
	storeSeenReply(post.id)
	if (!document.hidden) {
		return
	}
	repliedToMe()

	if ("vibrate" in navigator) {
		navigator.vibrate(1000)
	}

	if (
		isMobile
		|| !options.notification
		|| Notification.permission !== "granted"
	) {
		return
	}

	let icon: string
	if (!options.hideThumbs && !options.workModeToggle) {
		if (post.image) {
			const {SHA1, fileType} = post.image
			icon = thumbPath(SHA1, fileType)
		} else {
			icon = defaultIcon
		}
	}
	const n = new Notification(lang.quoted, {
		icon,
		body: post.body,
	})
	n.onclick = () => {
		n.close()
		window.focus()
		location.hash = "#p" + post.id

		// Next animation frame is delayed, when tab has no focus. Must scroll
		// only after render for there to be an element to scroll to.
		read(() =>
			write(scrollToAnchor))
	}
}
