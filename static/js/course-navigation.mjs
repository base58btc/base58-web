const sessionFields = ["courseSlug", "courseAttempt", "courseVersion", "draftPreview", "draftVersion", "localPreview", "courseSource"];

export function sameCourseSession(current, next) {
  return Boolean(current?.courseSlug && next?.courseNavigation === "1" &&
    sessionFields.every(key => (current[key] || "") === (next[key] || "")));
}

export function isLessonURL(target, current, slug) {
  const mode = url => { const query = new URLSearchParams(url.search); query.delete("section"); return query.toString(); };
  return target.origin === current.origin && mode(target) === mode(current) &&
    target.pathname.startsWith("/courses/" + encodeURIComponent(slug) + "/");
}

// Only the latest requested page may commit, even if an older fetch ignores
// cancellation or finishes while we are waiting for a running cell to save.
export class NavigationRequest {
  begin() {
    this.current?.abort();
    const controller = new AbortController();
    this.current = controller;
    return {signal:controller.signal, active:() => this.current === controller && !controller.signal.aborted};
  }
}

export function installCourseNavigation({beforeNavigate, beforeSwap, afterSwap, leave, setBusy}) {
  const requests = new NavigationRequest();
  let displayedURL = new URL(location.href), busy = false;
  const shell = () => document.querySelector(".course-platform_shell");
  const notice = document.getElementById("course-navigation-status");
  const positions = new Map();
  if (!history.state?.courseLesson?.key) {
    history.replaceState({...history.state, courseLesson:{key:crypto.randomUUID()}}, "", location.href);
  }
  let displayedState = history.state;
  const showNotice = message => { notice.textContent = message; notice.hidden = !message; };
  const markBusy = value => {
    busy = value; setBusy(value);
    shell().inert = value;
    shell().setAttribute("aria-busy", String(value));
  };
  const saveScroll = () => {
    if (busy || location.href !== displayedURL.href) return;
    positions.set(history.state?.courseLesson?.key, [scrollX,scrollY]);
    displayedState = history.state;
  };
  saveScroll();
  history.scrollRestoration = "manual";
  window.addEventListener("scroll", saveScroll, {passive:true});

  async function fullNavigation(url, pop, request) {
    await beforeNavigate();
    if (!request.active()) return;
    await leave();
    if (!request.active()) return;
    if (pop) location.replace(url.href);
    else location.assign(url.href);
  }

  async function navigate(href, {pop = false, scroll = [0,0]} = {}) {
    const url = new URL(href, displayedURL);
    if (!pop) saveScroll();
    const request = requests.begin();
    markBusy(true); showNotice("Loading lesson…");
    try {
      if (!isLessonURL(url, displayedURL, shell().dataset.courseSlug)) {
        await fullNavigation(url, pop, request); return;
      }
      const response = await fetch(url, {credentials:"same-origin", cache:"no-store", signal:request.signal, headers:{Accept:"text/html"}});
      if (!request.active()) return;
      if (!response.ok) throw new Error("Unable to load lesson (" + response.status + "). Try the link again.");
      const html = new DOMParser().parseFromString(await response.text(), "text/html");
      if (!request.active()) return;
      const next = html.querySelector(".course-platform_shell");
      // A sign-in redirect, new attempt/version, or preview-mode change needs a
      // full navigation. Never attach a different course session to this VM.
      if (!sameCourseSession(shell().dataset, next?.dataset)) {
        await fullNavigation(new URL(response.url || url.href), pop, request); return;
      }
      showNotice("Finishing current work…");
      await beforeNavigate();
      if (!request.active()) return;
      beforeSwap();
      shell().replaceWith(document.importNode(next, true));
      document.title = html.title;
      if (!pop) history.pushState({courseLesson:{key:crypto.randomUUID()}}, "", url.href);
      displayedURL = url;
      displayedState = history.state;
      document.body.classList.remove("no-scroll");
      afterSwap();
      const heading = shell().querySelector("[data-course-section]:not([hidden]) h1") || shell().querySelector("h1");
      if (heading) { heading.tabIndex = -1; heading.focus({preventScroll:true}); }
      let anchor;
      try { anchor = url.hash && document.getElementById(decodeURIComponent(url.hash.slice(1))); } catch { /* Invalid hash: use the page top. */ }
      if (anchor) anchor.scrollIntoView();
      else window.scrollTo(...scroll);
      showNotice("");
    } catch (error) {
      if (!request.active()) return;
      if (pop) history.replaceState(displayedState, "", displayedURL.href);
      showNotice(error.message || "Unable to load lesson. Try again.");
    } finally {
      if (request.active()) markBusy(false);
    }
  }

  document.addEventListener("click", event => {
    const link = event.target.closest("a[href]");
    if (!link || link.target || link.hasAttribute("download") || event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    const url = new URL(link.href, location.href);
    if (!["http:","https:"].includes(url.protocol) || (url.pathname === displayedURL.pathname && url.search === displayedURL.search && url.origin === displayedURL.origin)) return;
    event.preventDefault();
    navigate(url.href);
  });
  window.addEventListener("popstate", event => {
    if (location.pathname === displayedURL.pathname && location.search === displayedURL.search) {
      // Back followed quickly by Forward can return to the still-visible page
      // while the first fetch is pending. That fetch must no longer commit.
      requests.begin();
      markBusy(false); showNotice("");
      displayedURL = new URL(location.href);
      const position = positions.get(event.state?.courseLesson?.key);
      if (position) window.scrollTo(...position);
      return;
    }
    navigate(location.href, {pop:true, scroll:positions.get(event.state?.courseLesson?.key) || [0,0]});
  });
  window.addEventListener("hashchange", () => {
    if (location.pathname === displayedURL.pathname && location.search === displayedURL.search) displayedURL = new URL(location.href);
  });
  return navigate;
}
