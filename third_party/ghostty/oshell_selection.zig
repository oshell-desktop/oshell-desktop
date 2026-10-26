//! OShell v0.2.92 SelectionService coordinates. These are logical buffer
//! positions, not pointers into retained terminal storage. Explicit history
//! truncation does not change them; output recycling and in-place scrolling
//! notify the selection separately.
const std = @import("std");

pub const Position = struct {
    col: usize,
    row: usize,

    pub fn before(a: Position, b: Position) bool {
        return a.row < b.row or (a.row == b.row and a.col < b.col);
    }

    fn equal(a: Position, b: Position) bool {
        return a.row == b.row and a.col == b.col;
    }
};

pub const Range = struct {
    start: Position,
    end: Position,
};

/// Private ABI 55 coordinate bridge; no borrowed storage pointers.
pub const CState = extern struct {
    start_col: u32,
    start_row: u32,
    end_col: u32,
    end_row: u32,
    active: u32,
    pivot_col: u32 = 0,
    pivot_row: u32 = 0,
    word_start_col: u32 = 0,
    word_start_row: u32 = 0,
    word_end_col: u32 = 0,
    word_end_row: u32 = 0,
    row_anchor: u32 = 0,
    flags: u32 = 0,
    pub fn fromState(v: State) CState {
        var result: CState = .{ .start_col = @intCast(v.start.col), .start_row = @intCast(v.start.row), .end_col = @intCast(v.end.col), .end_row = @intCast(v.end.row), .active = @intFromBool(v.active) };
        if (v.pivot) |p| {
            result.flags |= 1;
            result.pivot_col = @intCast(p.col);
            result.pivot_row = @intCast(p.row);
        }
        if (v.word_anchor) |w| {
            result.flags |= 2;
            result.word_start_col = @intCast(w.start.col);
            result.word_start_row = @intCast(w.start.row);
            result.word_end_col = @intCast(w.end.col);
            result.word_end_row = @intCast(w.end.row);
        }
        if (v.row_anchor) |r| {
            result.flags |= 4;
            result.row_anchor = @intCast(r);
        }
        return result;
    }
    pub fn toState(v: CState) State {
        return .{ .active = v.active != 0, .start = .{ .col = v.start_col, .row = v.start_row }, .end = .{ .col = v.end_col, .row = v.end_row }, .pivot = if (v.flags & 1 != 0) .{ .col = v.pivot_col, .row = v.pivot_row } else null, .word_anchor = if (v.flags & 2 != 0) .{ .start = .{ .col = v.word_start_col, .row = v.word_start_row }, .end = .{ .col = v.word_end_col, .row = v.word_end_row } } else null, .row_anchor = if (v.flags & 4 != 0) v.row_anchor else null };
    }
};

pub const State = struct {
    active: bool = false,
    start: Position = .{ .col = 0, .row = 0 },
    end: Position = .{ .col = 0, .row = 0 },
    pivot: ?Position = null,
    word_anchor: ?Range = null,
    row_anchor: ?usize = null,

    pub fn ordered(self: State) Range {
        return if (Position.before(self.end, self.start))
            .{ .start = self.end, .end = self.start }
        else
            .{ .start = self.start, .end = self.end };
    }

    pub fn clear(self: *State) void {
        self.active = false;
        self.pivot = null;
        self.word_anchor = null;
        self.row_anchor = null;
    }

    fn translated(pos: Position, top: usize, bottom: usize, lines: i64) ?Position {
        if (pos.row < top or pos.row > bottom) return pos;
        const row = @as(i128, @intCast(pos.row)) - lines;
        if (row < top or row > bottom) return null;
        return .{ .col = pos.col, .row = @intCast(row) };
    }

    /// Mirrors SelectionService.adjustForInPlaceScroll. Positive lines move
    /// content up. A selection crossing either boundary is discarded whole.
    pub fn scroll(self: *State, top: usize, bottom: usize, lines: i64) void {
        if (!self.active or lines == 0) return;
        const range = self.ordered();
        if (range.start.row > bottom or range.end.row < top) return;
        if (range.start.row < top or range.end.row > bottom) {
            self.clear();
            return;
        }
        const start = translated(self.start, top, bottom, lines) orelse {
            self.clear();
            return;
        };
        const end = translated(self.end, top, bottom, lines) orelse {
            self.clear();
            return;
        };
        var pivot = self.pivot;
        if (pivot) |p| {
            if (Position.equal(p, self.start) or Position.equal(p, self.end)) {
                pivot = translated(p, top, bottom, lines) orelse {
                    self.clear();
                    return;
                };
            }
        }
        var word = self.word_anchor;
        if (word) |w| {
            const a = translated(w.start, top, bottom, lines) orelse {
                self.clear();
                return;
            };
            const b = translated(w.end, top, bottom, lines) orelse {
                self.clear();
                return;
            };
            word = .{ .start = a, .end = b };
        }
        var row = self.row_anchor;
        if (row) |r| {
            row = (translated(.{ .col = 0, .row = r }, top, bottom, lines) orelse {
                self.clear();
                return;
            }).row;
        }
        self.start = start;
        self.end = end;
        self.pivot = pivot;
        self.word_anchor = word;
        self.row_anchor = row;
    }

    /// Column-restricted edits use a strict comparison even though the
    /// selection's end is half-open: an endpoint touching the left margin
    /// still invalidates the range in the reference implementation.
    pub fn columnScroll(self: *State, top: usize, bottom: usize, left: usize, right: usize) void {
        if (!self.active) return;
        const range = self.ordered();
        if (range.start.row > bottom or range.end.row < top) return;
        if (range.start.row == range.end.row and
            (range.end.col < left or range.start.col > right)) return;
        self.clear();
    }

    /// The source copy path clips the end row at read time, retaining its
    /// original column. Never mutate the stored logical range to fit storage.
    /// Out-of-storage starts have no readable text; do not index cyclic slots.
    pub fn copyRange(self: State, rows: usize) ?Range {
        if (!self.active or rows == 0) return null;
        var range = self.ordered();
        if (Position.equal(range.start, range.end) or range.start.row >= rows) return null;
        range.end.row = @min(range.end.row, rows - 1);
        return range;
    }

    /// TerminalView.selectedColumnsRange, expressed as a half-open interval.
    /// Highlighting uses the untrimmed logical range, unlike copy extraction.
    pub fn columns(self: State, row: usize, cols: usize) ?[2]usize {
        if (!self.active or cols == 0) return null;
        const a = self.start;
        const b = self.end;
        var start: usize = 0;
        var end: usize = 0;
        if (a.row == b.row and a.row == row) {
            if (a.col < b.col) {
                start = a.col;
                end = b.col + @intFromBool(b.col == cols - 1);
            } else if (a.col > b.col) {
                start = b.col;
                end = a.col;
            }
        } else {
            const range = self.ordered();
            if (row < range.start.row or row > range.end.row) return null;
            start = if (row == range.start.row) range.start.col else 0;
            end = if (row == range.end.row)
                range.end.col + @intFromBool(range.end.col == cols - 1)
            else
                cols;
        }
        start = @min(start, cols);
        end = @min(end, cols);
        return if (start < end) .{ start, end } else null;
    }
};

test "OShell explicit trimming keeps logical selection for later growth" {
    const testing = std.testing;
    const s: State = .{ .active = true, .start = .{ .col = 1, .row = 6 }, .end = .{ .col = 3, .row = 6 } };
    try testing.expect(s.copyRange(5) == null);
    try testing.expectEqual(Range{ .start = s.start, .end = s.end }, s.copyRange(7).?);
    try testing.expectEqual(@as(usize, 6), s.start.row);
    try testing.expectEqual(@as(usize, 6), s.end.row);
}

test "OShell clipped copy and visible highlight use different end columns" {
    const testing = std.testing;
    const s: State = .{ .active = true, .start = .{ .col = 1, .row = 2 }, .end = .{ .col = 3, .row = 6 } };
    const copied = s.copyRange(5).?;
    try testing.expectEqual(Position{ .col = 3, .row = 4 }, copied.end);
    try testing.expectEqual([2]usize{ 0, 12 }, s.columns(4, 12).?);
    try testing.expectEqual(Position{ .col = 3, .row = 6 }, s.end);
}

test "OShell storage-independent positions survive explicit truncation then scroll" {
    const testing = std.testing;
    var s: State = .{ .active = true, .start = .{ .col = 1, .row = 6 }, .end = .{ .col = 3, .row = 6 } };
    // Both endpoints outside the recycled range stay untouched.
    s.scroll(0, 4, 1);
    try testing.expect(s.active);
    try testing.expectEqual(@as(usize, 6), s.start.row);
    // A range straddling the current buffer must cancel instead of clipping.
    s.start.row = 2;
    s.scroll(0, 4, 1);
    try testing.expect(!s.active);
}

test "OShell scroll translates only an endpoint pivot and all word anchors" {
    const testing = std.testing;
    var s: State = .{ .active = true, .start = .{ .col = 1, .row = 2 }, .end = .{ .col = 3, .row = 3 }, .pivot = .{ .col = 2, .row = 2 }, .row_anchor = 2 };
    s.word_anchor = .{ .start = s.start, .end = .{ .col = 4, .row = 2 } };
    s.scroll(1, 4, 1);
    try testing.expectEqual(@as(usize, 1), s.start.row);
    try testing.expectEqual(@as(usize, 2), s.pivot.?.row);
    try testing.expectEqual(@as(usize, 1), s.word_anchor.?.start.row);
    try testing.expectEqual(@as(?usize, 1), s.row_anchor);
    s.pivot = s.start;
    s.scroll(0, 4, -1);
    try testing.expectEqual(s.start, s.pivot.?);
}

test "OShell lost auxiliary anchor cancels the complete selection" {
    const testing = std.testing;
    var s: State = .{ .active = true, .start = .{ .col = 1, .row = 2 }, .end = .{ .col = 3, .row = 2 }, .row_anchor = 1 };
    s.scroll(1, 4, 1);
    try testing.expect(!s.active);
    try testing.expect(s.pivot == null and s.word_anchor == null and s.row_anchor == null);
}

test "OShell margin comparison and reversed last-column highlight" {
    const testing = std.testing;
    var s: State = .{ .active = true, .start = .{ .col = 0, .row = 2 }, .end = .{ .col = 2, .row = 2 } };
    s.columnScroll(1, 3, 2, 6);
    try testing.expect(!s.active);
    s.active = true;
    s.end.col = 1;
    s.columnScroll(1, 3, 2, 6);
    try testing.expect(s.active);
    s.start.col = 0;
    s.end.col = 11;
    try testing.expectEqual([2]usize{ 0, 12 }, s.columns(2, 12).?);
    std.mem.swap(Position, &s.start, &s.end);
    try testing.expectEqual([2]usize{ 0, 11 }, s.columns(2, 12).?);
}
