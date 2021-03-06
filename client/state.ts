// Stores the state of the web application

import { emitChanges, ChangeEmitter } from './model'
import { Post } from './posts/models'
import PostCollection from './posts/collection'
import { getClosestID } from './util'
import { readIDs, storeID } from './db'
import { write } from './render'
import { send } from './connection'

// Server-wide global configurations
interface Configs extends ChangeEmitter {
	radio: boolean
	hats: boolean
	illyaDance: boolean
	captcha: boolean
	mature: boolean // Website intended for mature audiences
	defaultLang: string
	defaultCSS: string
	FAQ: string
	captchaPublicKey: string
	links: { [key: string]: string }
}

// Board-specific configurations
export interface BoardConfigs extends ChangeEmitter {
	readOnly: boolean
	textOnly: boolean
	forcedAnon: boolean
	hashCommands: boolean
	spoilers: boolean     // Text spoilers
	codeTags: boolean
	spoiler: string       //Image spoiler
	title: string
	notice: string
	rules: string
	banners: string[]
	[index: string]: any
}

// The current state of a board or thread page
export interface PageState extends ChangeEmitter {
	thread: number
	lastN: number
	board: string
	href: string
}

const thirtyDays = 30 * 24 * 60 * 60 * 1000,
	loading = document.getElementById('loading-image')

// Configuration passed from the server. Some values can be changed during
// runtime.
export const config: Configs = (window as any).config

// Currently existing boards
export let boards: string[] = (window as any).boards

// Indicates, if in mobile mode. Determined server-side.
export const isMobile: boolean = (window as any).isMobile

export let boardConfig: BoardConfigs = emitChanges({} as BoardConfigs)

// Load initial page state
export const page = emitChanges<PageState>(read(location.href))

// All posts currently displayed
export const posts = new PostCollection([])

// Posts I made in any tab
export let mine: Set<number>

// Replies to this user's posts the user has already seen
export let seenReplies: Set<number>

// Explicitly hidden posts and threads
export let hidden: Set<number>

// Debug mode with more verbose logging
export let debug: boolean = /[\?&]debug=true/.test(location.href)

// Read page state by parsing a URL
export function read(href: string): PageState {
	const noHash = href.split("#")[0],
		board = noHash.match(/\/(\w+)\//)[1],
		thread = noHash.match(/\/(\d+)(?:[\?&]\w+=\w+)*$/),
		lastN = noHash.match(/[\?&]last=(\d+)/)
	return {
		href,
		board: decodeURIComponent(board),
		thread: thread ? parseInt(thread[1]) : 0,
		lastN: lastN ? parseInt(lastN[1]) : 0,
	} as PageState
}

// Load post number sets from the database
export function loadFromDB(): Promise<Set<number>[]> {
	return Promise.all([
		readIDs("mine").then(ids =>
			mine = new Set(ids)),
		readIDs("seen").then(ids =>
			seenReplies = new Set(ids)),
		readIDs("hidden").then((ids) =>
			hidden = new Set(ids)),
	])
}

// Store the ID of a post this client created
export function storeMine(id: number) {
	mine.add(id)
	storeID("mine", id, thirtyDays)
}

// Store the ID of a post that replied to one of the user's posts
export function storeSeenReply(id: number) {
	seenReplies.add(id)
	storeID("seen", id, thirtyDays)
}

// Store the ID of a post or thread to hide
export function storeHidden(id: number) {
	hidden.add(id)
	storeID("hidden", id, thirtyDays)
}

// Retrieve model of closest parent post
export function getModel(el: Element): Post {
	const id = getClosestID(el)
	if (!id) {
		return null
	}
	return posts.get(id)
}

// Display or hide the loading animation
export function displayLoading(display: boolean) {
	write(() =>
		loading.style.display = display ? 'block' : 'none')
}

; (window as any).debugMode = () => {
	debug = true
		; (window as any).send = send
}
