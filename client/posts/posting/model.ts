// Logic for manipulating the views and FSM of post authoring and communicated
// the results to the server

import { message, send, handlers } from "../../connection"
import { OP, Post, TextState, ThreadData, ImageData, PostData } from "../models"
import { FormView, OPFormView } from "./view"
import { posts, storeMine } from "../../state"
import { postSM, postEvent, postState } from "./main"
import { applyMixins, extend } from "../../util"
import PostView from "../view"
import { SpliceResponse } from "../../client"
import { FileData } from "./upload"
import { newAllocRequest, PostCredentials } from "./identity"
import { write } from "../../render"

// A message created while disconnected for later sending
type BufferedMessage = [message, any]

interface PostCreationRequest extends PostCredentials {
	image?: FileData
	body?: string
}

// Form Model of an OP post
export class OPFormModel extends OP implements FormModel {
	sentAllocRequest: boolean
	bodyLength: number
	parsedLines: number
	view: FormView
	inputState: TextState
	messageBuffer: BufferedMessage[]

	abandon: () => void
	addReference: (id: number) => void
	commitChar: (char: string) => void
	commitBackspace: () => void
	commitClose: () => void
	commitSplice: (val: string) => void
	flushBuffer: () => void
	init: () => void
	lastBodyLine: () => string
	parseInput: (val: string) => void
	reformatInput: (val: string) => void
	requestAlloc: (body: string | null, image: FileData | null) => void
	send: (type: message, msg: any) => void

	constructor(id: number) {
		storeMine(id)

		const oldModel = posts.get(id) as OP,
			oldView = oldModel.view
		oldView.unbind()

		// Copy the parent model's state and data
		super(extractAttrs(oldModel) as ThreadData)

		// Replace old model and view pair with the postForm pair
		posts.addOP(this)
		const view = new OPFormView(this)
		oldView.el.replaceWith(view.el)

		postSM.feed(postEvent.hijack, { view, model: this })
		this.sentAllocRequest = true

		this.init()
	}
}

// Form model for regular reply posts
export class ReplyFormModel extends Post implements FormModel {
	isAllocated: boolean
	sentAllocRequest: boolean
	bodyLength: number
	parsedLines: number
	view: FormView
	inputState: TextState
	messageBuffer: BufferedMessage[]

	abandon: () => void
	addReference: (id: number) => void
	commitChar: (char: string) => void
	commitBackspace: () => void
	commitClose: () => void
	commitSplice: (val: string) => void
	flushBuffer: () => void
	init: () => void
	lastBodyLine: () => string
	parseInput: (val: string) => void
	reformatInput: (val: string) => void
	send: (type: message, msg: any) => void

	constructor() {
		super({
			id: 0,
			editing: true,
			time: Date.now(),
			body: "",
			state: {
				spoiler: false,
				quote: false,
				iDice: 0,
			},
		})

		this.init()
	}

	// Request allocation of a draft post to the server
	requestAlloc(body: string | null, image: FileData | null) {
		this.sentAllocRequest = true
		const req = newAllocRequest() as PostCreationRequest

		if (body) {
			req.body = body
			this.body = body
			this.bodyLength = body.length
			this.inputState.line = body
			this.reformatInput(body)
		}

		if (image) {
			req.image = image
		}

		send(message.insertPost, req)
		handlers[message.postID] = (id: number) => {
			this.setID(id)
			delete handlers[message.postID]
		}
	}

	// Set post ID and add to the post collection
	setID(id: number) {
		this.id = id
		postSM.feed(postEvent.alloc)
		posts.add(this)
	}

	// Handle draft post allocation
	onAllocation(data: PostData) {
		// May sometimes be called multiple times, because of reconnects
		if (this.isAllocated) {
			return
		}

		this.isAllocated = true
		extend(this, data)
		this.view.renderAlloc()
		storeMine(data.id)
		if (data.image) {
			this.insertImage(this.image)
		}
	}

	// Upload the file and request its allocation
	async uploadFile(file?: File) {
		// Already have image
		if (this.image) {
			return
		}

		write(() =>
			this.view.cancel.remove())

		const data = await this.view.uploadFile(file)

		// Upload failed or image added while thumbnailing
		if (!data || this.image) {
			return
		}

		if (!this.sentAllocRequest) {
			this.requestAlloc(null, data)
		} else {
			send(message.insertImage, data)
		}
	}

	// Insert the uploaded image into the model
	insertImage(img: ImageData) {
		this.image = img
		this.view.insertImage()
	}
}

// Override mixin for post authoring models
export class FormModel {
	sentAllocRequest: boolean
	bodyLength: number        // Compound length of the input text body
	parsedLines: number       // Number of closed, committed and parsed lines
	body: string
	view: PostView & FormView
	state: TextState          // State of the underlying normal post model

	// State of line being edited. Must be separated to not affect the
	// asynchronous updates of committed lines
	inputState: TextState

	// Buffer for messages committed during connection outage
	messageBuffer: BufferedMessage[]

	closePost: () => void
	spliceLine: (line: string, msg: SpliceResponse) => string
	resetState: () => void
	requestAlloc: (body: string | null, image: FileData | null) => void

	// Initialize state
	init() {
		this.bodyLength = this.parsedLines = 0
		this.inputState = {
			quote: false,
			spoiler: false,
			iDice: 0, // Not used in FormModel. TypeScript demands it.
			line: "",
		}
		this.messageBuffer = []
	}

	// Append a character to the model's body and reparse the line, if it's a
	// newline
	append(code: number) {
		const char = String.fromCodePoint(code)
		if (char === "\n") {
			this.view.terminateLine(this.parsedLines++)
		}
		this.body += char
	}

	// Remove the last character from the model's body
	backspace() {
		this.body = this.body.slice(0, -1)
	}

	// Splice the last line of the body
	splice(msg: SpliceResponse) {
		this.spliceLine(this.lastBodyLine(), msg)
	}

	// Compare new value to old and generate appropriate commands
	parseInput(val: string): void {
		const old = this.inputState.line

		// Rendering hack shenanigans - ignore
		if (old === val) {
			return
		}

		const lenDiff = val.length - old.length,
			exceeding = this.bodyLength + lenDiff - 2000

		// If exceeding max body length, shorten the value, trim input and try
		// again
		if (exceeding > 0) {
			this.view.trimInput(exceeding)
			return this.parseInput(val.slice(0, -exceeding))
		}

		if (!this.sentAllocRequest) {
			return this.requestAlloc(val, null)
		}

		if (lenDiff === 1 && val.slice(0, -1) === old) {
			return this.commitChar(val.slice(-1))
		}
		if (lenDiff === -1 && old.slice(0, -1) === val) {
			return this.commitBackspace()
		}

		return this.commitSplice(val, lenDiff)
	}

	// Commit a character appendage to the end of the line to the server
	commitChar(char: string) {
		this.bodyLength++
		if (char === "\n") {
			this.resetState()
			this.view.startNewLine()
			this.inputState.line = ""
		} else {
			this.inputState.line += char
		}
		this.send(message.append, char.codePointAt(0))
	}

	// Optionally buffer all data, if currently disconnected
	send(type: message, msg: any) {
		if (postSM.state === postState.halted) {
			this.messageBuffer.push([type, msg])
		} else {
			send(type, msg)
		}
	}

	// Flush any buffered messages to the server
	flushBuffer() {
		for (let [type, msg] of this.messageBuffer) {
			send(type, msg)
		}
		this.messageBuffer = []
	}

	// Send a message about removing the last character of the line to the
	// server
	commitBackspace() {
		this.inputState.line = this.inputState.line.slice(0, -1)
		this.bodyLength--
		this.send(message.backspace, null)
	}

	// Commit any other input change that is not an append or backspace
	commitSplice(v: string, lenDiff: number) {
		// Convert to arrays of chars to deal with multibyte unicode chars
		const old = Array.from(this.inputState.line),
			val = Array.from(v)
		let start: number

		// Find first differing character
		for (let i = 0; i < old.length; i++) {
			if (old[i] !== val[i]) {
				start = i
				break
			}
		}

		// New string is appended to the end
		if (start === undefined) {
			start = old.length
		}

		// Right now we simply resend the entire corrected string, including the
		// common part, because I can't figure out a diff algorithm that covers
		// all cases. The backend technically supports the latter.
		const end = val.slice(start).join("")
		this.send(message.splice, {
			start,
			len: -1,
			text: end,
		})
		this.bodyLength += lenDiff
		this.inputState.line = old.slice(0, start).join("") + end
		this.reformatInput(this.inputState.line)
	}

	// Reformat the text, if the input contains newlines
	reformatInput(val: string) {
		if (val.indexOf("\n") === -1) {
			return
		}
		const lines = val.split("\n"),
			lastLine = lines[lines.length - 1]
		this.inputState.line = lastLine
		this.view.injectLines(lines.slice(0, -1), lastLine)
	}

	// Close the form and revert to regular post
	commitClose() {
		// Normalize state
		this.state.line = this.inputState.line
		this.view.cleanUp()
		this.send(message.closePost, null)
	}

	// Turn post form into a regular post, because it has expired after a
	// period of posting ability loss
	abandon() {
		this.state.line = this.inputState.line
		this.view.cleanUp()
		this.closePost()
	}

	// Return the last line of the body
	lastBodyLine(): string {
		const i = this.body.lastIndexOf("\n")
		return this.body.slice(i + 1)
	}

	// Add a link to the target post in the input
	addReference(id: number) {
		let s = ""
		const {line} = this.inputState

		// If already linking a post, put the new one on the next line
		if (/^>>\d+ ?$/.test(line)) {
			s += "\n"
		} else if (line && line[line.length - 1] !== " ") {
			s += " "
		}

		s += `>>${id} `
		this.view.replaceLine(this.inputState.line + s)
	}
}

applyMixins(OPFormModel, FormModel)
applyMixins(ReplyFormModel, FormModel)

// Extract all non-function attributes from a model
function extractAttrs(src: { [key: string]: any }): { [key: string]: any } {
	const attrs: { [key: string]: any } = {}
	for (let key in src) {
		if (typeof src[key] !== "function") {
			attrs[key] = src[key]
		}
	}
	return attrs
}
