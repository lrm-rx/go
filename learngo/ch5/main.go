package main

import "fmt"

func main() {
	// go语言提供了哪些集合类型的数据结构, 数组/切片/map/list
	// 数组 定义: var name [count]int
	//var courses1 [3]string // courses类型, 数组 只有3个元素的数组
	//var courses2 [4]string
	//courses1 = courses2 // 类型不一样
	// []string 和 [3]string  // 这是两种不同的类型 前者是切片,后者是数组
	//courses1[0] = "go"
	//courses1[1] = "java"
	//courses1[2] = "react"

	//fmt.Println(courses1)
	//fmt.Printf("%T\r\n", courses1)
	//fmt.Printf("%T", courses2)

	//for _, value := range courses1 {
	//	fmt.Println(value)
	//}

	// 数组的初始化1
	courses1 := [3]string{"go", "java", "js"}
	//for _, value := range courses1 {
	//	fmt.Println(value)
	//}
	// 数组的初始化2
	//courses2 := [3]string{2: "vue"}
	//for _, value := range courses2 {
	//	fmt.Println(value)
	//}

	// 数组的初始化3
	courses3 := [...]string{"go", "java", "js"}
	for i := 0; i < len(courses3); i++ {
		fmt.Println(courses3[i])
	}

	//for _, value := range courses3 {
	//	fmt.Println(value)
	//}

	if courses1 == courses3 {
		fmt.Println("equal")
	}

	// 多维数组
	var coursesInfo [3][4]string
	coursesInfo[0] = [4]string{"go", "1h", "ming", "go体系课"}
	coursesInfo[1] = [4]string{"java", "2h", "ning", "java体系课"}
	coursesInfo[2] = [4]string{"js", "2.5h", "uzi", "js体系课"}

	for i := 0; i < len(coursesInfo); i++ {
		for j := 0; j < len(coursesInfo[i]); j++ {
			fmt.Print(coursesInfo[i][j] + "\t")
		}
		fmt.Println()
	}

	for _, row := range coursesInfo {
		fmt.Print(row)
		//for _, column := range row {
		//	fmt.Print(column + "\t")
		//}
		//fmt.Println()
	}

}
