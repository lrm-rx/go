package main

import (
	"fmt"
	"math"
)

// Shape 形状接口，声明周长与面积方法
type Shape interface {
	Perimeter() float64
	Area() float64
}

// Rectangle 矩形结构体：宽度与高度
type Rectangle struct {
	Width  float64
	Height float64
}

// Circular 圆结构体：半径
type Circular struct {
	Radius float64
}

// Perimeter Rectangle实现周长的方法
func (R Rectangle) Perimeter() float64 {
	return (R.Width + R.Height) * 2
}

// Area Rectangle实现面积的方法
func (R Rectangle) Area() float64 {
	return R.Width * R.Height
}

// Perimeter Circular实现周长的方法
func (C Circular) Perimeter() float64 {
	return math.Pi * (C.Radius * 2)
}

// Area Circular实现面积的方法
func (C Circular) Area() float64 {
	return math.Pi * (C.Radius * C.Radius)
}
func main() {
	var shape Shape = Rectangle{4.5, 5}
	fmt.Printf("%T,%#v,%#v\n", shape, shape.Perimeter(), shape.Area()) //main.Rectangle,19,22.5

	var shape2 Shape = Circular{6}
	fmt.Printf("%T,%.2f,%.2f", shape2, shape2.Perimeter(), shape2.Area()) //main.Circular,37.70,113.10
}
