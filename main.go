package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "math/big"
    "math/rand"
    "os"
    "runtime"
    "strings"
    "sync"
    "time"

    "github.com/btcsuite/btcd/btcec/v2"
    "github.com/btcsuite/btcd/btcutil"
    "github.com/btcsuite/btcd/chaincfg"
)

var (
    targetAddr   = "1BY8GQbnueYofwSuFAT3USAhGjPrkxDdW9"
    start        = new(big.Int)
    end          = new(big.Int)
    totalRange   = new(big.Int)
    chunkSize    = new(big.Int)
    stopChan     = make(chan struct{})
    mu           sync.Mutex
    progressFile = "andamento.txt"
    startTime    time.Time
    saveInterval = big.NewInt(1000000) 
)

func init() {
    var success bool
    start.SetString("4000000001f9d9e5a", 16)
    end, success = new(big.Int).SetString("7ffffffffffffffff", 16)
    if !success {
        fmt.Println("Erro ao definir o valor final do intervalo.")
        os.Exit(1)
    }
    totalRange = new(big.Int).Sub(end, start)
    chunkSize.SetString("1000000000", 16)
}

func readProgress(threadID int) *big.Int {
    content, err := ioutil.ReadFile(progressFile)
    if err != nil {
        fmt.Println("Nenhum progresso anterior encontrado, iniciando do começo.")
        return nil
    }

    lines := strings.Split(string(content), "\n")
    for _, line := range lines {
        if strings.HasPrefix(line, fmt.Sprintf("Thread %d: ", threadID)) {
            lastHex := strings.TrimPrefix(line, fmt.Sprintf("Thread %d: ", threadID))
            lastValue := new(big.Int)
            lastValue.SetString(lastHex, 16)
            fmt.Printf("Thread %d retomando do valor: %s\n", threadID, lastHex)
            return lastValue
        }
    }
    return nil
}

func saveProgress(threadID int, lastKey *big.Int) {
    mu.Lock()
    defer mu.Unlock()

    content, err := ioutil.ReadFile(progressFile)
    if err != nil && !os.IsNotExist(err) {
        fmt.Printf("Erro ao ler o progresso existente: %v\n", err)
        return
    }

    var newContent string
    lines := strings.Split(string(content), "\n")
    threadFound := false
    for i, line := range lines {
        if strings.HasPrefix(line, fmt.Sprintf("Thread %d: ", threadID)) {
            lines[i] = fmt.Sprintf("Thread %d: %064x", threadID, lastKey)
            threadFound = true
            break
        }
    }
    if !threadFound {
        lines = append(lines, fmt.Sprintf("Thread %d: %064x", threadID, lastKey))
    }
    newContent = strings.Join(lines, "\n")

    err = ioutil.WriteFile(progressFile, []byte(newContent), 0644)
    if err != nil {
        fmt.Printf("Erro ao salvar o progresso: %v\n", err)
    }
}

func showMenu() {
    fmt.Println("Escolha o modo de busca:")
    fmt.Println("1. Linear")
    fmt.Println("2. Aleatório")
    fmt.Println("3. Percentual")
    fmt.Println("4. Busca Fracionada")
    fmt.Print("Opção: ")

    var choice int
    fmt.Scanln(&choice)

    if choice < 1 || choice > 4 {
        fmt.Println("Opção inválida.")
        os.Exit(1)
    }

    clearScreen()
    startTime = time.Now()
    switch choice {
    case 1:
        startSearch(false)
    case 2:
        startSearch(true)
    case 3:
        startSearchByPercentage()
    case 4:
        startFractionalSearch()
    }
}

func startSearch(random bool) {
    var wg sync.WaitGroup
    numThreads := runtime.NumCPU()

    fmt.Printf("Iniciando modo %s com %d threads.\n", map[bool]string{false: "Linear", true: "Aleatório"}[random], numThreads)

    for i := 0; i < numThreads; i++ {
        wg.Add(1)
        go searchKeys(start, end, targetAddr, i, &wg, random)
    }

    wg.Wait()
}

func startSearchByPercentage() {
    var percentage float64
    fmt.Print("Digite o percentual de início (0 a 100): ")
    fmt.Scanln(&percentage)

    if percentage < 0 || percentage > 100 {
        fmt.Println("Percentual inválido.")
        return
    }

    rangePercentage := new(big.Float).Quo(big.NewFloat(percentage), big.NewFloat(100))
    startPercentage := new(big.Float).Mul(new(big.Float).SetInt(totalRange), rangePercentage)
    startInt := new(big.Int)
    startPercentage.Int(startInt)

    fmt.Printf("Iniciando a busca a partir do percentual %.20f%%.\n", percentage)
    var wg sync.WaitGroup
    numThreads := runtime.NumCPU()

    for i := 0; i < numThreads; i++ {
        wg.Add(1)
        go searchKeys(startInt, end, targetAddr, i, &wg, false)
    }

    wg.Wait()
}

func startFractionalSearch() {
    var wg sync.WaitGroup
    numThreads := 100  // Definido para 100 threads
    partSize := new(big.Int).Div(totalRange, big.NewInt(int64(numThreads)))  // Divide em 100 partes

    fmt.Printf("Iniciando modo fracionado com %d threads.\n", numThreads)

    for i := 0; i < numThreads; i++ {
        wg.Add(1)
        
        threadStart := new(big.Int).Add(start, new(big.Int).Mul(partSize, big.NewInt(int64(i))))
        threadEnd := new(big.Int).Add(threadStart, partSize)

        if i == numThreads-1 {
            threadEnd = end
        }

        go searchKeys(threadStart, threadEnd, targetAddr, i, &wg, false)
    }

    wg.Wait()
    fmt.Println("Busca fracionada concluída.")
}

func searchKeys(start, end *big.Int, targetAddr string, threadID int, wg *sync.WaitGroup, random bool) {
    defer wg.Done()

    keyInt := new(big.Int).Set(start)
    savedValue := readProgress(threadID)
    if savedValue != nil {
        keyInt = savedValue
    }

    rangeSize := new(big.Int).Sub(end, start)
    rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
    
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for keyInt.Cmp(end) <= 0 {
        select {
        case <-stopChan:
            return
        case <-ticker.C:
            fmt.Printf("Thread %d: Progresso atual: %064x\n", threadID, keyInt)
        default:
        }

        if random {
            keyInt.Rand(rnd, rangeSize).Add(keyInt, start)
        }

        privKey, _ := btcec.PrivKeyFromBytes(keyInt.Bytes())
        pubKey := privKey.PubKey()

        addr, err := btcutil.NewAddressPubKey(pubKey.SerializeCompressed(), &chaincfg.MainNetParams)
        if err != nil {
            log.Printf("Erro ao criar o endereço: %v", err)
            continue
        }

        if addr.EncodeAddress() == targetAddr {
            fmt.Printf("Chave privada encontrada! %064x\n", keyInt)
            saveKeyToFile(fmt.Sprintf("%064x", keyInt))
            close(stopChan)
            return
        }

        if new(big.Int).Mod(keyInt, saveInterval).Cmp(big.NewInt(0)) == 0 {
            saveProgress(threadID, keyInt)
        }

        keyInt.Add(keyInt, big.NewInt(1))
    }
}

func saveKeyToFile(key string) {
    content := fmt.Sprintf("Chave privada encontrada: %s\n", key)
    err := ioutil.WriteFile("found_key.txt", []byte(content), 0644)
    if err != nil {
        fmt.Printf("Erro ao salvar a chave encontrada: %v\n", err)
    } else {
        fmt.Println("Chave privada salva em found_key.txt.")
    }
}

func clearScreen() {
    fmt.Print("\033[H\033[2J")
}

func main() {
    showMenu()
}
