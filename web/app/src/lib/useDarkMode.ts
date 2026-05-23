import { useEffect, useState } from "react";

// Tracks the site theme, which Topbar toggles by adding/removing the "dark"
// class on <html>. Used to bind the CodeMirror theme to the active palette.
export function useDarkMode(): boolean {
  const [dark, setDark] = useState(() => document.documentElement.classList.contains("dark"));
  useEffect(() => {
    const el = document.documentElement;
    const observer = new MutationObserver(() => {
      setDark(el.classList.contains("dark"));
    });
    observer.observe(el, { attributes: true, attributeFilter: ["class"] });
    return () => observer.disconnect();
  }, []);
  return dark;
}
