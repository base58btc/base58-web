(function(root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.CourseSections = api;
})(typeof window !== "undefined" ? window : globalThis, function() {
  "use strict";

  function sectionSlug(title, used) {
    const base = title.toLowerCase().normalize("NFKD").replace(/[\u0300-\u036f]/g, "")
      .replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "section";
    let slug = base, suffix = 2;
    while (used.has(slug)) slug = base + "-" + suffix++;
    used.add(slug);
    return slug;
  }

  function sectionURL(href, section) {
    const url = new URL(href, location.href);
    url.hash = "";
    if (section && section !== "overview") url.searchParams.set("section", section);
    else url.searchParams.delete("section");
    return url.pathname + url.search;
  }

  const mounted = new WeakMap();
  function mount(shell, {signal, onChange = () => {}} = {}) {
    if (mounted.has(shell)) return mounted.get(shell)();
    const article = shell.querySelector(".course-platform_article");
    const headings = [...article.children].filter(node => node.tagName === "H2");
    const chapterLink = [...shell.querySelectorAll(".course-platform_nav-link[data-lesson-path]")]
      .find(link => link.dataset.lessonPath === shell.dataset.lessonPath);
    if (headings.length < 2 || !chapterLink) return;

    const chapterNumber = chapterLink.querySelector(".course-platform_nav-number").textContent;
    const mobile = shell.querySelector(".course-platform_mobile-toc-current");
    const originalLabel = mobile?.textContent || "";
    const chapterURL = sectionURL(location.href, "overview");
    const originalTitle = document.title;
    const pager = shell.querySelector(".course-platform_pager");
    const previous = pager.querySelector(".previous"), next = pager.querySelector(".next");
    const adjacent = link => link && ({url:link.href, title:link.querySelector(".course-platform_pager-title").textContent});
    const previousChapter = adjacent(previous), nextChapter = adjacent(next);
    const sections = [], used = new Set(["overview", "last"]);
    const startSection = (slug, title) => {
      const element = document.createElement("section");
      element.dataset.courseSection = slug;
      if (slug === "overview") element.dataset.overview = "true";
      article.append(element);
      const section = {slug, title, element};
      sections.push(section);
      return section;
    };
    const nodes = [...article.childNodes];
    let current = startSection("overview", "Overview");
    for (const node of nodes) {
      if (node.nodeType === 1 && node.tagName === "H2") {
        current = startSection(sectionSlug(node.textContent, used), node.textContent);
        const title = document.createElement("h1");
        for (const attribute of node.attributes) title.setAttribute(attribute.name, attribute.value);
        title.append(...node.childNodes);
        if (!title.id) title.id = current.slug;
        node.replaceWith(title);
        current.element.append(title);
      } else current.element.append(node);
    }

    const links = document.createElement("div");
    links.className = "course-platform_subpages course-platform_sections";
    for (const [index, section] of sections.entries()) {
      const link = document.createElement("a"), number = document.createElement("span"), title = document.createElement("span");
      link.className = "course-platform_nav-link";
      link.href = sectionURL(chapterURL, section.slug);
      number.className = "course-platform_nav-number";
      number.textContent = chapterNumber + "." + String(index).padStart(2, "0");
      title.className = "course-platform_nav-title";
      title.textContent = section.title;
      link.append(number, title); links.append(link);
      section.link = link;
    }
    chapterLink.after(links);
    chapterLink.classList.remove("is-current");
    chapterLink.classList.add("is-active");
    chapterLink.removeAttribute("aria-current");

    function pagerLink(direction, destination) {
      if (!destination) return;
      const link = document.createElement("a"), label = document.createElement("span"), title = document.createElement("span");
      link.className = "course-platform_pager-link " + direction;
      link.href = destination.url;
      label.className = "course-platform_pager-label";
      label.textContent = direction === "previous" ? "Previous" : "Next";
      title.className = "course-platform_pager-title";
      title.textContent = destination.title;
      link.append(label, title); pager.append(link);
    }
    function show() {
      let anchor;
      try { anchor = location.hash && document.getElementById(decodeURIComponent(location.hash.slice(1))); } catch { /* Ignore malformed fragments. */ }
      const requested = new URL(location.href).searchParams.get("section");
      let index = sections.findIndex(section => anchor && section.element.contains(anchor));
      if (index < 0) index = requested === "last" ? sections.length - 1 : sections.findIndex(section => section.slug === requested);
      if (index < 0) index = 0;
      for (const [i, section] of sections.entries()) {
        section.element.hidden = i !== index;
        section.link.classList.toggle("is-current", i === index);
        if (i === index) section.link.setAttribute("aria-current", "page");
        else section.link.removeAttribute("aria-current");
      }
      const section = sections[index];
      shell.dataset.section = section.slug;
      if (mobile) mobile.textContent = index === 0 ? originalLabel :
        chapterNumber + "." + String(index).padStart(2, "0") + " / " + section.title;
      document.title = index === 0 ? originalTitle : section.title + " · " + originalTitle;
      const before = index > 0 ? {url:sectionURL(chapterURL, sections[index-1].slug), title:sections[index-1].title} :
        previousChapter && {url:sectionURL(previousChapter.url, "last"), title:previousChapter.title};
      const after = index + 1 < sections.length ? {url:sectionURL(chapterURL, sections[index+1].slug), title:sections[index+1].title} : nextChapter;
      pager.replaceChildren(); pagerLink("previous", before); pagerLink("next", after);
      shell.dataset.prevUrl = before?.url || "";
      shell.dataset.nextUrl = after?.url || "";
    }
    mounted.set(shell, show);
    show();
    window.addEventListener("hashchange", () => { show(); onChange(); }, {signal});
  }
  return {mount};
});
