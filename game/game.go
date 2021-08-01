package game

import (
	"bufio"
	"fmt"
	"math"
	"os"
)

type Game struct {
	LevelChans []chan *Level
	InputChan  chan *Input
	Level      *Level
}

type InputType int

const (
	None InputType = iota
	Up
	Down
	Left
	Right
	Search
	QuitGame
	CloseWindow
)

type Input struct {
	Typ       InputType
	MousePos  Pos
	LevelChan chan *Level
}

var OffsetX int32
var OffsetY int32

type Tile rune

const (
	PlayerTile Tile = '@'
	StoneWall  Tile = '#'
	DirtFloor  Tile = '.'
	ClosedDoor Tile = '|'
	OpenDoor   Tile = '/'
	Rat        Tile = 'R'
	Spider     Tile = 'S'
	Empty      Tile = 0
)

type Pos struct {
	X, Y int32
}

type Entity struct {
	Pos
}

type Player struct {
	Entity
}

type Level struct {
	Zone     [][]Tile
	Player   Player
	Monsters map[Pos]*Monster
	Debug    map[Pos]bool
}

type priorityPos struct {
	Pos
	priority int
}

func NewGame(numWindows int, path string) *Game {
	levelChans := make([]chan *Level, numWindows)
	for i := range levelChans {
		levelChans[i] = make(chan *Level)
	}
	inputChan := make(chan *Input)

	return &Game{LevelChans: levelChans, InputChan: inputChan, Level: loadLevelFromFile(path)}
}

func loadLevelFromFile(filename string) *Level {
	file, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	level := &Level{}
	scanner := bufio.NewScanner(file)
	zoneRows := make([]string, 0)
	longestRow := 0
	index := 0

	for scanner.Scan() {
		zoneRows = append(zoneRows, scanner.Text())
		if len(zoneRows[index]) > longestRow {
			longestRow = len(zoneRows[index])
		}
		index++
	}
	level.Zone = make([][]Tile, len(zoneRows))
	level.Monsters = make(map[Pos]*Monster)

	for i := range level.Zone {
		level.Zone[i] = make([]Tile, longestRow)
	}
	for y := 0; y < len(level.Zone); y++ {
		line := zoneRows[y]
		for x, r := range line {
			var t Tile
			switch r {
			case ' ', '\n', '\t', '\r':
				t = Empty
			case '#':
				t = StoneWall
			case '.':
				t = DirtFloor
			case '|':
				t = ClosedDoor
			case '/':
				t = OpenDoor
			case '@':
				level.Player.X = int32(x)
				level.Player.Y = int32(y)
				t = DirtFloor
			case 'R':
				level.Monsters[Pos{int32(x), int32(y)}] = NewRat()
				t = Rat
			case 'S':
				level.Monsters[Pos{int32(x), int32(y)}] = NewSpider()
				t = Spider
			default:
				panic(fmt.Sprintf("Invalid rune '%s' in map at position [%d,%d]", string(r), y+1, x+1))
			}
			level.Zone[y][x] = t
		}
	}

	return level
}

func canWalk(level *Level, pos Pos) bool {
	t := level.Zone[pos.Y][pos.X]
	switch t {
	case StoneWall, Empty:
		return false
	default:
		return true
	}
}

func checkDoor(level *Level, pos Pos) {
	t := level.Zone[pos.Y][pos.X]
	if t == ClosedDoor {
		level.Zone[pos.Y][pos.X] = OpenDoor
	}
}

func (g *Game) handleInput(input *Input) {
	level := g.Level
	p := level.Player
	switch input.Typ {
	case Up:
		if canWalk(level, Pos{p.X, p.Y - 1}) {
			level.Player.Y--
		} else {
			checkDoor(level, Pos{p.X, p.Y - 1})
		}
	case Down:
		if canWalk(level, Pos{p.X, p.Y + 1}) {
			level.Player.Y++
		} else {
			checkDoor(level, Pos{p.X, p.Y + 1})
		}
	case Left:
		if canWalk(level, Pos{p.X - 1, p.Y}) {
			level.Player.X--
		} else {
			checkDoor(level, Pos{p.X - 1, p.Y})
		}
	case Right:
		if canWalk(level, Pos{p.X + 1, p.Y}) {
			level.Player.X++
		} else {
			checkDoor(level, Pos{p.X + 1, p.Y})
		}
	case Search:
		pos := screenToWorldPos(input.MousePos)
		t := g.getTileAtPos(pos)
		if t == DirtFloor {
			g.astar(p.Pos, input.MousePos)
		} else if t == ClosedDoor {
			level.Zone[pos.Y][pos.X] = OpenDoor
		}
	case CloseWindow:
		close(input.LevelChan)
		chanIndex := 0
		for i, c := range g.LevelChans {
			if c == input.LevelChan {
				chanIndex = i
				break
			}
		}
		g.LevelChans = append(g.LevelChans[:chanIndex], g.LevelChans[chanIndex+1:]...)
	case None:
		break
	}
}

func (g *Game) bfsearch(start Pos) {
	level := g.Level
	edge := make([]Pos, 0, 8)
	edge = append(edge, start)
	visited := make(map[Pos]bool)
	visited[start] = true
	level.Debug = visited

	for len(edge) > 0 {
		current := edge[0]
		edge = edge[1:]
		for _, next := range getNeighbours(level, current) {
			if !visited[next] {
				edge = append(edge, next)
				visited[next] = true
			}
		}
	}
}

func (g *Game) astar(start Pos, goal Pos) []Pos {
	level := g.Level
	goal = screenToWorldPos(goal)
	fmt.Printf("start: {%d, %d}\ngoal: {%d, %d}\n", start.X, start.Y, goal.X, goal.Y)
	edge := make(pqueue, 0, 8)
	edge = edge.push(start, 1)
	prevPos := make(map[Pos]Pos)
	prevPos[start] = start
	currentCost := make(map[Pos]int)
	currentCost[start] = 0

	level.Debug = make(map[Pos]bool)

	var current Pos
	for len(edge) > 0 {
		edge, current = edge.pop()

		if current == goal {
			path := make([]Pos, 0)
			p := current
			for p != start {
				path = append(path, p)
				p = prevPos[p]
			}
			path = append(path, p)
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			level.Debug = make(map[Pos]bool)
			for _, pos := range path {
				level.Debug[pos] = true
			}

			return path
		}

		for _, next := range getNeighbours(level, current) {
			var cost = 1
			t := g.getTileAtPos(next)
			if t == ClosedDoor {cost = 4}
			newCost := currentCost[current] + cost
			_, exists := currentCost[next]
			if !exists || newCost < currentCost[next] {
				currentCost[next] = newCost
				xDist := int(math.Abs(float64(goal.X - next.X)))
				yDist := int(math.Abs(float64(goal.Y - next.Y)))
				priority := newCost + xDist + yDist
				edge = edge.push(next, priority)
				level.Debug[next] = true
				prevPos[next] = current
				fmt.Printf("{%d, %d} to {%d, %d} cost: %d\n",current.X, current.Y, next.X, next.Y, newCost)
			}
		}
	}

	return nil
}

func getNeighbours(level *Level, pos Pos) []Pos {
	neighbours := make([]Pos, 0, 4)
	u := Pos{pos.X, pos.Y - 1}
	d := Pos{pos.X, pos.Y + 1}
	l := Pos{pos.X - 1, pos.Y}
	r := Pos{pos.X + 1, pos.Y}

	if canWalk(level, u) {
		neighbours = append(neighbours, u)
	}
	if canWalk(level, d) {
		neighbours = append(neighbours, d)
	}
	if canWalk(level, l) {
		neighbours = append(neighbours, l)
	}
	if canWalk(level, r) {
		neighbours = append(neighbours, r)
	}

	return neighbours
}

func (g *Game) getTileAtPos(pos Pos) Tile {
	return g.Level.Zone[pos.Y][pos.X]
}

func screenToWorldPos(pos Pos) Pos {
	return Pos{(pos.X-OffsetX)/32, (pos.Y-OffsetY)/32}
}

func (g *Game) Run() {
	fmt.Println("Starting...")

	for _, lchan := range g.LevelChans {
		lchan <- g.Level
	}

	for input := range g.InputChan {
		if input.Typ == QuitGame {
			return
		}
		g.handleInput(input)

		if len(g.LevelChans) == 0 {
			return
		}
		for _, lchan := range g.LevelChans {
			lchan <- g.Level
		}
	}
}
