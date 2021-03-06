import { escape } from '../util'
import { ThreadData, PostData, Post, OP } from '../posts/models'
import PostView, { OPView } from '../posts/view'
import { page, posts as postCollection, hidden } from '../state'
import { threads, importTemplate } from '../render'
import options from "../options"
import { setTitle } from "../tab"
import { expandAll } from "../posts/images"
import { images as lang } from "../lang"
import { renderNotice } from "./common"
import { updateSyncTimestamp } from "../connection"

// Container for all rendered posts
export let threadContainer: HTMLElement

// Render the HTML of a thread page
export default function renderThread(thread: ThreadData) {
	updateSyncTimestamp()
	const frag = importTemplate("thread")

	// Apply title to header and tab
	const title = `/${page.board}/ - ${escape(thread.subject)} (#${thread.id})`
	setTitle(title)
	frag.querySelector("h1").innerHTML = title

	threadContainer = frag.querySelector("#thread-container")
	if (!options.workModeToggle && (options.userBG || options.illyaDance)) {
		threadContainer.classList.add("custom-BG")
	}
	const els: Element[] = [],
		{posts} = thread
	delete thread.posts // Reduce strain on the GC. We won't be using these.

	// Render larger thumbnail for the OP
	if (thread.image) {
		thread.image.large = true
	}

	frag.querySelector("#expand-images")
		.textContent = expandAll ? lang.contract : lang.expand

	const opModel = new OP(thread),
		opView = new OPView(opModel)
	els.push(opView.el)
	postCollection.addOP(opModel)
	postCollection.lowestID = posts.length ? posts[0].id : opModel.id

	for (let post of posts) {
		if (!hidden.has(post.id)) {
			els.push(createPost(post))
		}
	}
	threadContainer.append(...els)

	renderNotice(frag)

	if (page.lastN) {
		opView.renderOmit()
	}

	threads.innerHTML = ""
	threads.append(frag)
}

function createPost(data: PostData): Element {
	const model = new Post(data),
		view = new PostView(model)
	postCollection.add(model)
	return view.el
}
