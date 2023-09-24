package main

import "fmt"

func main() {
	// go 折中 slice 切片 - 数组
	//var courses []string
	//fmt.Printf("%T\r\n", courses)
	//
	//courses = append(courses, "go")
	//courses = append(courses, "java")
	//courses = append(courses, "js")
	//fmt.Println(courses)
	//fmt.Println(courses[0])
	//
	//// 切片的初始化 3种: 1. 从数组直接创建  2.使用string{}  3. make
	//allCourses1 := [5]string{"go", "js", "java", "mysql", "react"}
	//courseSlice := allCourses1[0:len(allCourses1)] // 左闭右开 [)  -- 包头不包尾
	//fmt.Println(courseSlice)
	//
	//allCourses2 := []string{"js", "go", "java", "mysql", "react"}
	//fmt.Println(allCourses2)

	// make 函数  超出长度也会报错
	//allCourses3 := make([]string, 3)
	//allCourses3[0] = "c"
	//fmt.Println(allCourses3)

	// 以下报错
	//var allCourses4 []string
	//allCourses4[0] = "c"

	// 访问切片的元素, 访问单个, 访问多个
	//fmt.Println(allCourses2[0])
	/*
		[start:end]
				1. 如果只有start, 没有end, 表示从start开始到结尾的所有数据
				2. 如果没有start, 有end, 表示从0开始到end之前的所有数据
				3. 如果没有start, 也没有end, 表示所有数据
				4. 如果有start, 也有end, 表示左闭右开 [)  -- 包头不包尾
	*/
	//fmt.Println(allCourses2[1:4])
	//fmt.Println(allCourses2[:])
	//fmt.Println(allCourses2[:4])
	//fmt.Println(allCourses2[3:])

	//allCourses2 := []string{"js", "go"}
	//allCourses2 = append(allCourses2, "java", "vue")
	//allCourses3 := []string{"react", "jquery", "es"}
	//allCourses2 = append(allCourses2, allCourses3[1:]...)
	//for _, value := range allCourses3 {
	//	allCourses2 = append(allCourses2, value)
	//}
	//fmt.Println(allCourses2)

	// 如何删除slice中的元素: 比较麻烦
	courseSlice := []string{"js", "go", "mysql", "react", "jquery"}
	//myslice := append(courseSlice[:2], courseSlice[3:]...)
	//fmt.Println(myslice)
	//
	//courseSlice = courseSlice[:3]
	//fmt.Println(courseSlice)

	// 复制slice
	//courseSliceCopy := courseSlice
	courseSliceCopy2 := courseSlice[:]
	//fmt.Println(courseSliceCopy)
	fmt.Println(courseSliceCopy2)

	var courseSliceCopy = make([]string, len(courseSlice))
	copy(courseSliceCopy, courseSlice)
	fmt.Println(courseSliceCopy)
	fmt.Println("---------------")
	courseSlice[0] = "java"
	fmt.Println(courseSliceCopy2)
	fmt.Println(courseSliceCopy)
}
