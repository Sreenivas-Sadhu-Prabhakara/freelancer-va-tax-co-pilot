/* Quarterly income tax tracker for Philippine freelancers and virtual assistants.
   No dependencies. Every figure comes from the binary serving this page; this
   file formats what it is given and sends back what is typed. All arithmetic
   happens in the Go package, where it is tested. */
(function () {
  "use strict";

  var PESO = "₱";
  var state = null;
  var quarter = 1;

  function $(id) { return document.getElementById(id); }

  /* ---------------------------------------------------------- formatting */

  /* Amounts arrive as a whole number of centavos, so there is no floating
     point anywhere in this file. */
  function money(centavos) {
    var n = Number(centavos) || 0;
    var neg = n < 0;
    if (neg) { n = -n; }
    var whole = Math.floor(n / 100);
    var frac = n % 100;
    var s = String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, ",");
    s = s + "." + (frac < 10 ? "0" + frac : String(frac));
    return (neg ? "-" : "") + PESO + s;
  }

  function plain(centavos) {
    var n = Number(centavos) || 0;
    var neg = n < 0;
    if (neg) { n = -n; }
    return (neg ? "-" : "") + Math.floor(n / 100) + "." + (n % 100 < 10 ? "0" : "") + (n % 100);
  }

  function rate(basisPoints) {
    var bp = Number(basisPoints) || 0;
    return (bp / 100).toString().replace(/\.0+$/, "") + "%";
  }

  function longDate(iso) {
    if (!iso) { return ""; }
    var parts = String(iso).split("-");
    if (parts.length !== 3) { return String(iso); }
    var months = ["January", "February", "March", "April", "May", "June",
      "July", "August", "September", "October", "November", "December"];
    var m = months[Number(parts[1]) - 1] || parts[1];
    return Number(parts[2]) + " " + m + " " + parts[0];
  }

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) { node.className = cls; }
    if (text !== undefined && text !== null) { node.textContent = text; }
    return node;
  }

  function clear(node) { while (node.firstChild) { node.removeChild(node.firstChild); } }

  /* ---------------------------------------------------------- the server */

  function show(err) {
    var box = $("error");
    if (!err) { box.hidden = true; box.textContent = ""; return; }
    box.hidden = false;
    box.textContent = err;
  }

  function api(method, path, body) {
    var opts = { method: method, headers: { "Accept": "application/json" } };
    if (body !== undefined) {
      opts.headers["Content-Type"] = "application/json";
      opts.body = JSON.stringify(body);
    }
    document.body.classList.add("busy");
    return fetch(path, opts).then(function (res) {
      return res.json().catch(function () { return { error: "the tool replied with something unreadable" }; })
        .then(function (data) {
          if (!res.ok) { throw new Error(data && data.error ? data.error : "request failed"); }
          return data;
        });
    }).finally(function () { document.body.classList.remove("busy"); });
  }

  function load(path, body, method) {
    return api(method || "GET", path, body).then(function (data) {
      state = data;
      show(null);
      render();
      return data;
    }).catch(function (e) { show(e.message); });
  }

  function year() { return state ? state.year : new Date().getFullYear(); }

  /* ---------------------------------------------------------- rendering */

  function render() {
    if (!state) { return; }
    renderHead();
    renderElection();
    renderEntries();
    renderConstants();
    renderPosition();
    renderFigures();
    renderRecomputed();
    renderProjection();
    renderDeadlines();
    renderNotes();
  }

  function renderHead() {
    $("todayPill").textContent = "today " + longDate(state.today);
    var p = state.position;
    var q = p.quarters[quarter - 1];
    var regime = q.regime === "eight-percent" ? "8% flat rate" : "graduated rates";
    if (q.regime !== "eight-percent") {
      regime += q.deduction === "itemized" ? " · itemised" : " · standard deduction";
    }
    $("regimePill").textContent = regime;
    $("entryPill").textContent = state.entries.length + " entr" + (state.entries.length === 1 ? "y" : "ies");

    var sel = $("yearSelect");
    var years = (state.years || []).slice();
    var now = new Date().getFullYear();
    [now, now - 1, now + 1, state.year].forEach(function (y) {
      if (years.indexOf(y) < 0) { years.push(y); }
    });
    years.sort(function (a, b) { return b - a; });
    clear(sel);
    years.forEach(function (y) {
      var o = el("option", null, String(y));
      o.value = String(y);
      if (y === state.year) { o.selected = true; }
      sel.appendChild(o);
    });

    $("exportEntries").setAttribute("href", "/api/export?what=entries&year=" + state.year);
    $("exportFigures").setAttribute("href",
      "/api/export?what=figures&year=" + state.year + "&quarter=" + quarter);

    /* An entry has to fall inside the taxable year, so the date picker is
       held to it and starts on a day inside it. */
    var date = $("entryDate");
    var first = state.year + "-01-01";
    var last = state.year + "-12-31";
    date.min = first;
    date.max = last;
    if (!date.value || date.value < first || date.value > last) {
      date.value = (state.today >= first && state.today <= last) ? state.today : first;
    }
  }

  function pressed(wrapNode, attr, value) {
    var buttons = wrapNode.querySelectorAll("button[" + attr + "]");
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].setAttribute("aria-pressed", String(buttons[i].getAttribute(attr) === value));
    }
  }

  function renderElection() {
    var e = state.election;
    pressed($("regimeSeg"), "data-regime", e.regime);
    pressed($("deductionSeg"), "data-deduction", e.deduction);
    $("deductionField").hidden = e.regime === "eight-percent";
    $("hasCompensation").checked = !!e.hasCompensation;
    $("vatRegistered").checked = !!e.vatRegistered;
    $("priorYearExcess").value = e.priorYearExcess ? plain(e.priorYearExcess) : "";
    $("otherCredits").value = e.otherCredits ? plain(e.otherCredits) : "";
  }

  var KIND_NAMES = {
    receipt: "Receipt", return: "Return / refund",
    nonoperating: "Other income", expense: "Expense"
  };

  function renderEntries() {
    var body = $("entryTable").tBodies[0];
    clear(body);
    var entries = state.entries || [];
    $("entryCount").textContent = entries.length ? "(" + entries.length + ")" : "";
    $("entryEmpty").hidden = entries.length > 0;
    $("entryTable").hidden = entries.length === 0;

    entries.forEach(function (entry) {
      var tr = el("tr");
      tr.appendChild(el("td", "date", entry.date));
      var qn = Math.floor((Number(entry.date.split("-")[1]) - 1) / 3) + 1;
      tr.appendChild(el("td", "q", "Q" + qn));
      var what = KIND_NAMES[entry.kind] || entry.kind;
      if (entry.kind === "expense") {
        what += entry.category === "cost" ? " (cost of services)" : " (ordinary)";
      }
      tr.appendChild(el("td", "what", what));
      tr.appendChild(el("td", "what", [entry.client, entry.note].filter(Boolean).join(" — ")));
      tr.appendChild(el("td", "num", money(entry.amount)));
      tr.appendChild(el("td", "num", entry.withheld ? money(entry.withheld) : "—"));

      var cell = el("td", "drop");
      var btn = el("button", "drop-btn", "Remove");
      btn.type = "button";
      btn.setAttribute("aria-label", "Remove the entry of " + money(entry.amount) + " on " + entry.date);
      btn.addEventListener("click", function () {
        load("/api/entry?year=" + state.year + "&id=" + encodeURIComponent(entry.id), undefined, "DELETE");
      });
      cell.appendChild(btn);
      tr.appendChild(cell);
      body.appendChild(tr);
    });
  }

  function renderConstants() {
    var body = $("constantsTable").tBodies[0];
    clear(body);
    (state.constants || []).forEach(function (c) {
      var tr = el("tr");
      tr.appendChild(el("td", null, c.name));
      tr.appendChild(el("td", "num", c.value));
      var src = el("td", "what");
      src.appendChild(el("span", null, c.reference));
      if (c.confidence !== "sourced") {
        src.appendChild(el("span", "src", "could not be confirmed against the primary text"));
      }
      tr.appendChild(src);
      body.appendChild(tr);
    });
    $("overrideNote").hidden = !state.ratesOverridden;
    var r = state.rates;
    $("ovRate8").value = (r.eightPercentRateBasisPoints / 100).toString();
    $("ovAllowance").value = plain(r.eightPercentAllowance);
    $("ovThreshold").value = plain(r.vatThreshold);
    $("ovOSD").value = (r.osdRateBasisPoints / 100).toString();
  }

  function row(body, label, value, cls) {
    var tr = el("tr", cls || null);
    tr.appendChild(el("td", null, label));
    tr.appendChild(el("td", "num", value));
    body.appendChild(tr);
    return tr;
  }

  function renderPosition() {
    pressed($("quarterSeg"), "data-quarter", String(quarter));
    var p = state.position.quarters[quarter - 1];

    var label = p.isAnnual
      ? "Annual return for " + state.year
      : "Quarter " + quarter + " (" + p.period.months + "), year to date";
    var value = $("posValue");
    value.classList.remove("over");
    if (p.overpayment > 0) {
      $("posLabel").textContent = label + " · overpaid";
      value.textContent = money(p.overpayment);
      value.classList.add("over");
      $("posSub").textContent = "Nothing to pay. On the return this goes in brackets on the "
        + "“Tax Payable/(Overpayment)” line.";
    } else {
      $("posLabel").textContent = label + " · to pay";
      value.textContent = money(p.amountToPay);
      $("posSub").textContent = "Due " + longDate(p.deadline)
        + (p.deadlineIsWeekend ? " — a weekend; confirm the working date" : "")
        + " on BIR Form No. " + p.form + ".";
    }

    var body = $("positionTable").tBodies[0];
    clear(body);
    row(body, "Receipts, year to date", money(p.receipts));
    if (p.salesReturns) { row(body, "Less: returns, refunds and discounts", money(p.salesReturns), "sub"); }
    if (p.nonOperating) { row(body, "Add: other non-operating income", money(p.nonOperating), "sub"); }
    /* Gross is the same figure as receipts until something is taken off it or
       added to it, and a line that repeats the one above it is noise. */
    if (p.gross !== p.receipts) { row(body, "Gross, year to date", money(p.gross), "sub"); }

    if (p.regime === "eight-percent") {
      row(body, "Less: allowable reduction", money(p.allowance), "sub");
    } else {
      if (p.deduction === "itemized") {
        row(body, "Less: cost of sales or services", money(p.costOfServices), "sub");
        row(body, "Less: ordinary allowable deductions", money(p.itemizedDeduction), "sub");
      } else {
        row(body, "Less: optional standard deduction (" + rate(p.osdRateBasisPoints) + " of net sales)",
          money(p.osdAmount), "sub");
      }
      if (p.netIncome < 0) {
        row(body, "Net loss, year to date", money(p.netIncome), "sub");
      }
    }
    row(body, "Taxable income, year to date", money(p.taxable), "head");
    var rateText = p.regime === "eight-percent"
      ? "Tax due to date at " + rate(p.taxRateBasisPoints)
      : "Tax due to date (graduated, marginal " + rate(p.marginalRateBasisPoints) + ")";
    row(body, rateText, money(p.taxDue), "head");

    row(body, "Less: prior year's excess credits", money(p.priorYearExcess), "sub");
    var paidRow = row(body, p.isAnnual
      ? "Less: tax paid for the first three quarters"
      : "Less: tax paid in the earlier quarters", money(p.paidPriorQuarters), "sub");
    if (p.paidPriorQuartersAssumed && p.paidPriorQuarters > 0) {
      paidRow.cells[0].appendChild(el("span", "why", "this tool's own figure — record what you actually paid"));
    }
    row(body, "Less: tax withheld, earlier quarters", money(p.withheldPriorQuarters), "sub");
    row(body, "Less: tax withheld, this quarter", money(p.withheldThisQuarter), "sub");
    if (p.otherCredits) { row(body, "Less: other credits", money(p.otherCredits), "sub"); }
    row(body, "Total credits", money(p.totalCredits), "sub");
    row(body, "Tax payable / (overpayment)", money(p.taxPayable), "total");

    if (p.percentageTax) {
      var pt = p.percentageTax;
      row(body, "Percentage tax on this quarter's gross alone (2551Q)", money(pt.due), "head");
      row(body, rate(pt.rateBasisPoints) + " of " + money(pt.base) + ", due " + longDate(pt.deadline),
        "", "sub");
    }

    var recorded = state.election.payments[quarter - 1] || {};
    $("paidAmount").value = recorded.recorded ? plain(recorded.amount) : "";
    $("paidDate").value = recorded.filedOn || "";
    $("paidLabel").textContent = p.isAnnual
      ? "What you actually paid with the annual return"
      : "What you actually paid with the quarter " + quarter + " return";
  }

  function renderFigures() {
    var f = state.figures[quarter - 1];
    $("figuresHead").textContent = "Figures for BIR Form No. " + f.form;
    $("figuresCovers").textContent = "Covers " + f.covers + ". Due " + longDate(f.deadline)
      + ". Amounts are rounded to whole pesos, which is what the return asks for.";
    $("figuresCaveat").textContent = f.caveat;

    var host = $("figuresBody");
    clear(host);
    (f.blocks || []).forEach(function (block) {
      var wrapNode = el("div", "figblock");
      wrapNode.appendChild(el("h3", null, block.title));
      var table = el("table", "breakdown");
      var body = document.createElement("tbody");
      (block.lines || []).forEach(function (line) {
        var tr = el("tr", line.kind === "total" ? "total" : (line.kind === "amount" ? "sub" : null));
        var labelCell = el("td", null, line.label);
        if (line.item) { labelCell.appendChild(el("span", "item", " item " + line.item)); }
        if (line.note) { labelCell.appendChild(el("span", "why", line.note)); }
        tr.appendChild(labelCell);
        tr.appendChild(el("td", "num", line.kind === "choice" ? line.text : money(line.peso)));
        body.appendChild(tr);
      });
      table.appendChild(body);
      var scroll = el("div", "tablewrap");
      scroll.appendChild(table);
      wrapNode.appendChild(scroll);
      host.appendChild(wrapNode);
    });
  }

  function renderRecomputed() {
    var p = state.position;
    var card = $("recomputedCard");
    if (!p.recomputedWhy) { card.hidden = true; return; }
    card.hidden = false;
    $("recomputedWhy").textContent = p.recomputedWhy;
    var body = $("recomputedTable").tBodies[0];
    clear(body);
    (p.recomputed || []).forEach(function (q) {
      var tr = el("tr");
      tr.appendChild(el("td", null, q.isAnnual ? "Year" : "Q" + q.quarter));
      tr.appendChild(el("td", "num", money(q.taxable)));
      tr.appendChild(el("td", "num", money(q.taxDue)));
      tr.appendChild(el("td", "num", money(q.totalCredits)));
      tr.appendChild(el("td", "num", q.overpayment > 0 ? "(" + money(q.overpayment) + ")" : money(q.amountToPay)));
      body.appendChild(tr);
    });
  }

  function renderProjection() {
    var pr = state.position.projection;
    var card = $("projectionCard");
    if (!pr || !pr.available) { card.hidden = true; return; }
    card.hidden = false;
    var text = money(pr.grossToDate) + " of gross over " + pr.quartersCounted + " quarter"
      + (pr.quartersCounted === 1 ? "" : "s") + " annualises to " + money(pr.projectedGross)
      + ", on which the tax would be " + money(pr.projectedTax) + ".";
    if (pr.crossesThreshold) {
      text += " At that pace the year crosses the registration threshold, which ends the 8% option and"
        + " requires value-added tax registration.";
    }
    $("projectionText").textContent = text;
  }

  var STATUS_WORDS = { overdue: "overdue", today: "due today", soon: "due soon", upcoming: "upcoming" };

  function renderDeadlines() {
    var body = $("deadlineTable").tBodies[0];
    clear(body);
    (state.position.deadlines || []).forEach(function (d) {
      var tr = el("tr");
      tr.appendChild(el("td", null, d.due));
      tr.appendChild(el("td", "q", d.form));
      var what = el("td", "what", d.what);
      what.appendChild(el("span", "src", d.covers));
      tr.appendChild(what);
      var words = STATUS_WORDS[d.status] || d.status;
      if (d.status === "overdue") { words += " by " + (-d.daysAway) + " day" + (d.daysAway === -1 ? "" : "s"); }
      else if (d.status !== "today") { words += " · in " + d.daysAway + " day" + (d.daysAway === 1 ? "" : "s"); }
      if (d.isWeekend) { words += " (a weekend)"; }
      tr.appendChild(el("td", "what status-" + d.status, words));
      body.appendChild(tr);
    });
  }

  function renderNotes() {
    var list = $("notesList");
    clear(list);
    var seen = {};
    var all = (state.position.quarters[quarter - 1].notes || []).concat(state.position.notes || []);
    all.forEach(function (n) {
      if (seen[n.text]) { return; }
      seen[n.text] = true;
      var li = el("li", n.severity === "warn" ? "warn" : (n.severity === "stop" ? "stop" : null));
      li.appendChild(el("span", null, n.text));
      if (n.reference) { li.appendChild(el("span", "src", n.reference)); }
      list.appendChild(li);
    });
  }

  /* ---------------------------------------------------------- the wiring */

  $("yearSelect").addEventListener("change", function () {
    load("/api/year", { year: Number(this.value) }, "POST");
  });

  $("regimeSeg").addEventListener("click", function (e) {
    var b = e.target.closest("button[data-regime]");
    if (!b) { return; }
    state.election.regime = b.getAttribute("data-regime");
    renderElection();
  });

  $("deductionSeg").addEventListener("click", function (e) {
    var b = e.target.closest("button[data-deduction]");
    if (!b) { return; }
    state.election.deduction = b.getAttribute("data-deduction");
    renderElection();
  });

  $("saveElection").addEventListener("click", function () {
    load("/api/election", {
      year: year(),
      regime: state.election.regime,
      deduction: state.election.deduction,
      hasCompensation: $("hasCompensation").checked,
      vatRegistered: $("vatRegistered").checked,
      priorYearExcess: $("priorYearExcess").value,
      otherCredits: $("otherCredits").value
    }, "POST");
  });

  $("quarterSeg").addEventListener("click", function (e) {
    var b = e.target.closest("button[data-quarter]");
    if (!b) { return; }
    quarter = Number(b.getAttribute("data-quarter"));
    render();
  });

  $("entryKind").addEventListener("change", function () {
    var isExpense = this.value === "expense";
    $("categoryField").hidden = !isExpense;
    $("withheldField").hidden = isExpense;
  });

  $("entryForm").addEventListener("submit", function (e) {
    e.preventDefault();
    var kind = $("entryKind").value;
    load("/api/entry", {
      year: year(),
      date: $("entryDate").value,
      kind: kind,
      amount: $("entryAmount").value,
      client: $("entryClient").value,
      withheld: kind === "expense" ? "" : $("entryWithheld").value,
      category: kind === "expense" ? $("entryCategory").value : ""
    }, "POST").then(function () {
      $("entryAmount").value = "";
      $("entryClient").value = "";
      $("entryWithheld").value = "";
      $("entryAmount").focus();
    });
  });

  $("pasteToggle").addEventListener("click", function () {
    var box = $("pasteBox");
    box.hidden = !box.hidden;
    if (!box.hidden) { $("pasteText").focus(); }
  });

  $("pasteCancel").addEventListener("click", function () {
    $("pasteBox").hidden = true;
    $("pasteProblems").hidden = true;
  });

  $("pasteAdd").addEventListener("click", function () {
    var text = $("pasteText").value;
    if (!text.trim()) { return; }
    api("POST", "/api/entries", { year: year(), text: text }).then(function (data) {
      state = data;
      show(null);
      render();
      var list = $("pasteProblems");
      clear(list);
      var problems = data.problems || [];
      list.hidden = problems.length === 0;
      problems.forEach(function (p) { list.appendChild(el("li", null, p)); });
      if (problems.length === 0) {
        $("pasteText").value = "";
        $("pasteBox").hidden = true;
      }
    }).catch(function (e) { show(e.message); });
  });

  $("savePayment").addEventListener("click", function () {
    load("/api/payment", {
      year: year(), quarter: quarter, recorded: true,
      amount: $("paidAmount").value || "0", filedOn: $("paidDate").value
    }, "POST");
  });

  $("clearPayment").addEventListener("click", function () {
    load("/api/payment", { year: year(), quarter: quarter, recorded: false, amount: "0", filedOn: "" }, "POST");
  });

  $("saveRates").addEventListener("click", function () {
    var r = JSON.parse(JSON.stringify(state.rates));
    r.eightPercentRateBasisPoints = Math.round(Number($("ovRate8").value) * 100);
    r.osdRateBasisPoints = Math.round(Number($("ovOSD").value) * 100);
    r.eightPercentAllowance = Math.round(Number(String($("ovAllowance").value).replace(/,/g, "")) * 100);
    r.vatThreshold = Math.round(Number(String($("ovThreshold").value).replace(/,/g, "")) * 100);
    if (!isFinite(r.eightPercentRateBasisPoints) || !isFinite(r.osdRateBasisPoints)
      || !isFinite(r.eightPercentAllowance) || !isFinite(r.vatThreshold)) {
      show("one of those figures is not a number");
      return;
    }
    load("/api/rates", { year: year(), rates: r }, "POST");
  });

  $("resetRates").addEventListener("click", function () {
    load("/api/rates", { year: year(), reset: true }, "POST");
  });

  $("printBtn").addEventListener("click", function () { window.print(); });

  /* The theme toggle, the same one every tool in the family uses. */
  (function theme() {
    var root = document.documentElement;
    function current() {
      var t = root.getAttribute("data-theme");
      if (t) { return t; }
      return (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) ? "dark" : "light";
    }
    function paint(b) {
      var dark = current() === "dark";
      b.textContent = dark ? "☀ Light" : "☾ Dark";
      b.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
    }
    var btn = document.querySelector("[data-themer]");
    if (!btn) { return; }
    paint(btn);
    btn.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem("omsme-theme", next); } catch (e) { /* private window */ }
      paint(btn);
    });
  })();

  /* Open on the quarter whose return is next to be filed, which is the one
     somebody sitting down with this actually wants. */
  load("/api/state").then(function (data) {
    if (!data) { return; }
    var qs = data.position.quarters;
    quarter = qs.length;
    for (var i = 0; i < qs.length; i++) {
      if (qs[i].deadline >= data.today) { quarter = i + 1; break; }
    }
    render();
  });
})();
