/// <reference types="mdast" />
// @ts-nocheck
import { h } from "hastscript";

// GitHub language colors (subset of most common languages)
const LANGUAGE_COLORS = {
	JavaScript: "#f1e05a",
	TypeScript: "#3178c6",
	Python: "#3572A5",
	Java: "#b07219",
	"C++": "#f34b7d",
	C: "#555555",
	"C#": "#178600",
	Go: "#00ADD8",
	Rust: "#dea584",
	Ruby: "#701516",
	PHP: "#4F5D95",
	Swift: "#F05138",
	Kotlin: "#A97BFF",
	Dart: "#00B4AB",
	Scala: "#c22d40",
	Shell: "#89e051",
	HTML: "#e34c26",
	CSS: "#563d7c",
	SCSS: "#c6538c",
	Vue: "#41b883",
	Svelte: "#ff3e00",
	Astro: "#ff5a03",
	MDX: "#fcb32c",
	Markdown: "#083fa1",
	JSON: "#292929",
	YAML: "#cb171e",
	Dockerfile: "#384d54",
	Makefile: "#427819",
	Lua: "#000080",
	Perl: "#0298c3",
	R: "#198CE7",
	Julia: "#a270ba",
	Haskell: "#5e5086",
	Elixir: "#6e4a7e",
	Clojure: "#db5855",
	Objective_C: "#438eff",
	"Objective-C": "#438eff",
	Jupyter_Notebook: "#DA5B0B",
	"Jupyter Notebook": "#DA5B0B",
	Vim_Script: "#199f4b",
	TeX: "#3D6117",
	PowerShell: "#012456",
};

/**
 * Creates a GitHub Card component.
 *
 * @param {Object} properties - The properties of the component.
 * @param {string} properties.repo - The GitHub repository in the format "owner/repo".
 * @param {import('mdast').RootContent[]} children - The children elements of the component.
 * @returns {import('mdast').Parent} The created GitHub Card component.
 */
export function GithubCardComponent(properties, children) {
	if (Array.isArray(children) && children.length !== 0)
		return h("div", { class: "hidden" }, [
			'Invalid directive. ("github" directive must be leaf type "::github{repo="owner/repo"}")',
		]);

	if (!properties.repo || !properties.repo.includes("/"))
		return h(
			"div",
			{ class: "hidden" },
			'Invalid repository. ("repo" attributte must be in the format "owner/repo")',
		);

	const repo = properties.repo;
	const cardUuid = `GC${Math.random().toString(36).slice(-6)}`; // Collisions are not important

	const nAvatar = h(`div#${cardUuid}-avatar`, { class: "gc-avatar" });
	const nLanguage = h(
		`span#${cardUuid}-language`,
		{ class: "gc-language" },
		"—",
	);

	const nTitle = h("div", { class: "gc-titlebar" }, [
		h("div", { class: "gc-titlebar-left" }, [
			h("div", { class: "gc-owner" }, [
				nAvatar,
				h("div", { class: "gc-user" }, repo.split("/")[0]),
			]),
			h("div", { class: "gc-divider" }, "/"),
			h("div", { class: "gc-repo" }, repo.split("/")[1]),
		]),
		h("div", { class: "github-logo" }),
	]);

	const nDescription = h(
		`div#${cardUuid}-description`,
		{ class: "gc-description" },
		"Loading...",
	);

	const nStars = h(`div#${cardUuid}-stars`, { class: "gc-stars" }, "—");
	const nForks = h(`div#${cardUuid}-forks`, { class: "gc-forks" }, "—");
	const nLicense = h(`div#${cardUuid}-license`, { class: "gc-license" }, "—");

	const nScript = h(
		`script#${cardUuid}-script`,
		{ type: "text/javascript", defer: true },
		`
      const langColors = ${JSON.stringify(LANGUAGE_COLORS)};
      fetch('https://api.github.com/repos/${repo}', { referrerPolicy: "no-referrer" }).then(response => response.json()).then(data => {
        document.getElementById('${cardUuid}-description').innerText = data.description?.replace(/:[a-zA-Z0-9_]+:/g, '') || "No description provided";
        const lang = data.language || "";
        const langEl = document.getElementById('${cardUuid}-language');
        langEl.innerText = lang || "—";
        if (lang && langColors[lang]) {
          langEl.style.setProperty('--gc-lang-color', langColors[lang]);
        }
        document.getElementById('${cardUuid}-forks').innerText = Intl.NumberFormat('en-us', { notation: "compact", maximumFractionDigits: 1 }).format(data.forks).replaceAll("\u202f", '');
        document.getElementById('${cardUuid}-stars').innerText = Intl.NumberFormat('en-us', { notation: "compact", maximumFractionDigits: 1 }).format(data.stargazers_count).replaceAll("\u202f", '');
        const avatarEl = document.getElementById('${cardUuid}-avatar');
        avatarEl.style.backgroundImage = 'url(' + data.owner.avatar_url + ')';
        avatarEl.style.backgroundColor = 'transparent';
        document.getElementById('${cardUuid}-license').innerText = data.license?.spdx_id || "—";
        document.getElementById('${cardUuid}-card').classList.remove("fetch-waiting");
      }).catch(err => {
        const c = document.getElementById('${cardUuid}-card');
        c?.classList.add("fetch-error");
        console.warn("[GITHUB-CARD] Error loading card for ${repo}");
      })
    `,
	);

	return h(
		`a#${cardUuid}-card`,
		{
			class: "card-github fetch-waiting not-prose",
			href: `https://github.com/${repo}`,
			target: "_blank",
			rel: "noopener noreferrer",
			repo,
		},
		[
			nTitle,
			nDescription,
			h("div", { class: "gc-infobar" }, [nStars, nForks, nLicense, nLanguage]),
			nScript,
		],
	);
}
