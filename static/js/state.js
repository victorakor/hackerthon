var currentUser = null;
var authToken = null;
var questions = [];
var completedIds = new Set();
var currentFilter = 'all';
var currentQuestion = null;
var followingIds = new Set();
var starValues = {};

// Session timer
var timerSeconds = 5 * 60 * 60;
var timerRunning = false;
var timerInterval = null;

// Per-question timers: { questionId: remainingSeconds }
var questionTimers = {};
var questionTickInterval = null;