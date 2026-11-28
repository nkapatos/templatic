// The app entry: loaded on every page. Bootstrap comes from npm, so its
// CSS and JS are part of this bundle; Head links the fingerprinted files.
import "bootstrap/dist/css/bootstrap.css";
import "./app.css";

// Importing Bootstrap's JS wires its data API (dropdown, collapse, modal)
// on document; the hooks add what needs explicit init.
import "./tooltips";
import "./modal";
